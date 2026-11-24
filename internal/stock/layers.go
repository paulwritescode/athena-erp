package stock

import (
	"context"
	"database/sql"
	"fmt"

	"athena-erp/internal/posting"
	"athena-erp/internal/types"

	"github.com/google/uuid"
)

// layerOp moves quantity on one layer. Qty and Value are magnitudes; Kind is
// "issue" for consumption and "return" for restoration, and ReversesEdge
// names the edge a cancel mirror unwinds.
type layerOp struct {
	LayerID      string
	Qty          types.Decimal
	Value        types.Decimal
	ReversesEdge string
	Kind         string
}

// stockLayer is a receipt layer with its unconsumed residual.
type stockLayer struct {
	ID       string
	Residual types.Decimal
	Original types.Decimal
	UnitCost types.Decimal
}

// appEdge is one stored item application edge.
type appEdge struct {
	ID      string
	LayerID string
	Qty     types.Decimal
	Value   types.Decimal
}

// openLayers returns the layers with a residual for a bin's item, location
// and lot, oldest receipt first, locking them. Receipt row ids are UUIDv7,
// so id order is receipt order.
func openLayers(ctx context.Context, tx *sql.Tx, bin binKey) ([]stockLayer, error) {
	query := `SELECT id, residual_qty, original_qty, unit_cost FROM athena_stock_layers WHERE company_name = ? AND book = ? AND item_id = ? AND location_name = ? AND %s AND residual_qty > 0 ORDER BY receipt_row_id FOR UPDATE`
	lot := "lot_no IS NULL"
	args := []any{bin.Company, bin.Book, bin.ItemID, bin.Location}
	if bin.LotNo != "" {
		lot = "lot_no = ?"
		args = append(args, bin.LotNo)
	}
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(query, lot), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []stockLayer
	for rows.Next() {
		var layer stockLayer
		var residual, original, cost string
		if err := rows.Scan(&layer.ID, &residual, &original, &cost); err != nil {
			return nil, err
		}
		if layer.Residual, err = scanDecimal(residual); err != nil {
			return nil, err
		}
		if layer.Original, err = scanDecimal(original); err != nil {
			return nil, err
		}
		if layer.UnitCost, err = scanDecimal(cost); err != nil {
			return nil, err
		}
		out = append(out, layer)
	}
	return out, rows.Err()
}

// coverLayers consumes layers oldest first until magnitude is covered,
// valuing each take at the moving average. The last take absorbs rounding
// dust so the takes sum to total exactly. Layers short of the bin quantity
// are an integrity break: every valued receipt creates its layer.
func coverLayers(layers []stockLayer, magnitude, average, total types.Decimal) ([]layerOp, error) {
	var ops []layerOp
	remaining := magnitude
	valued := total
	for _, layer := range layers {
		if remaining.Sign() <= 0 {
			break
		}
		take := layer.Residual
		if take.Cmp(remaining) > 0 {
			take = remaining
		}
		remaining = remaining.Sub(take)
		ops = append(ops, layerOp{LayerID: layer.ID, Qty: take, Kind: "issue"})
	}
	if remaining.Sign() > 0 {
		return nil, fmt.Errorf("%w: layers cover %s of %s", ErrIntegrity, magnitude.Sub(remaining).String(), magnitude.String())
	}
	for i := range ops {
		if i == len(ops)-1 {
			ops[i].Value = valued
			break
		}
		edge, err := round6(ops[i].Qty.Mul(average))
		if err != nil {
			return nil, err
		}
		ops[i].Value = edge
		valued = valued.Sub(edge)
	}
	return ops, nil
}

// applicationEdges returns the non-reversal edges of a row and kind,
// locking them.
func applicationEdges(ctx context.Context, tx *sql.Tx, rowID, kind string) ([]appEdge, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, receipt_layer_id, applied_qty, applied_value FROM athena_item_applications WHERE issue_row_id = ? AND kind = ? AND is_reversal = 0 ORDER BY id FOR UPDATE`,
		rowID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []appEdge
	for rows.Next() {
		var edge appEdge
		var qty, value string
		if err := rows.Scan(&edge.ID, &edge.LayerID, &qty, &value); err != nil {
			return nil, err
		}
		if edge.Qty, err = scanDecimal(qty); err != nil {
			return nil, err
		}
		if edge.Value, err = scanDecimal(value); err != nil {
			return nil, err
		}
		out = append(out, edge)
	}
	return out, rows.Err()
}

// prorateRestore scales an issue's edges down to a partial return. A full
// return restores the exact edge amounts; a partial one prorates with the
// last edge absorbing dust so takes sum exactly.
func prorateRestore(edges []appEdge, origQty, returnQty, returnValue types.Decimal) ([]layerOp, error) {
	if len(edges) == 0 {
		return nil, fmt.Errorf("%w: issue has no applications to restore", ErrIntegrity)
	}
	full := returnQty.Cmp(origQty) == 0
	var ops []layerOp
	leftQty, leftValue := returnQty, returnValue
	for i, edge := range edges {
		op := layerOp{LayerID: edge.LayerID, Kind: "return"}
		if full {
			op.Qty, op.Value = edge.Qty, edge.Value
			ops = append(ops, op)
			continue
		}
		if i == len(edges)-1 {
			op.Qty, op.Value = leftQty, leftValue
			ops = append(ops, op)
			break
		}
		scaledQty, err := edge.Qty.Mul(returnQty).Div(origQty, LedgerScale+6, LedgerRounding)
		if err != nil {
			return nil, err
		}
		qty, err := round6(scaledQty)
		if err != nil {
			return nil, err
		}
		scaledValue, err := edge.Value.Mul(returnQty).Div(origQty, LedgerScale+6, LedgerRounding)
		if err != nil {
			return nil, err
		}
		value, err := round6(scaledValue)
		if err != nil {
			return nil, err
		}
		op.Qty, op.Value = qty, value
		leftQty, leftValue = leftQty.Sub(qty), leftValue.Sub(value)
		ops = append(ops, op)
	}
	return ops, nil
}

// layerByReceipt returns the layer a receipt row created, locking it.
func layerByReceipt(ctx context.Context, tx *sql.Tx, receiptRowID string) (stockLayer, bool, error) {
	var layer stockLayer
	var residual, original, cost string
	err := tx.QueryRowContext(ctx, `SELECT id, residual_qty, original_qty, unit_cost FROM athena_stock_layers WHERE receipt_row_id = ? FOR UPDATE`,
		receiptRowID).Scan(&layer.ID, &residual, &original, &cost)
	if err != nil {
		if err == sql.ErrNoRows {
			return stockLayer{}, false, nil
		}
		return stockLayer{}, false, err
	}
	var scanErr error
	if layer.Residual, scanErr = scanDecimal(residual); scanErr != nil {
		return stockLayer{}, false, scanErr
	}
	if layer.Original, scanErr = scanDecimal(original); scanErr != nil {
		return stockLayer{}, false, scanErr
	}
	if layer.UnitCost, scanErr = scanDecimal(cost); scanErr != nil {
		return stockLayer{}, false, scanErr
	}
	return layer, true, nil
}

// layerByID returns one layer by id, locking it.
func layerByID(ctx context.Context, tx *sql.Tx, id string) (stockLayer, bool, error) {
	var layer stockLayer
	layer.ID = id
	var residual, original, cost string
	err := tx.QueryRowContext(ctx, `SELECT residual_qty, original_qty, unit_cost FROM athena_stock_layers WHERE id = ? FOR UPDATE`,
		id).Scan(&residual, &original, &cost)
	if err != nil {
		if err == sql.ErrNoRows {
			return stockLayer{}, false, nil
		}
		return stockLayer{}, false, err
	}
	var scanErr error
	if layer.Residual, scanErr = scanDecimal(residual); scanErr != nil {
		return stockLayer{}, false, scanErr
	}
	if layer.Original, scanErr = scanDecimal(original); scanErr != nil {
		return stockLayer{}, false, scanErr
	}
	if layer.UnitCost, scanErr = scanDecimal(cost); scanErr != nil {
		return stockLayer{}, false, scanErr
	}
	return layer, true, nil
}

// application is one item application edge to store. Qty and Value are
// magnitudes; Kind with Reversal gives the direction.
type application struct {
	Company      string
	Book         string
	Event        string
	Kind         string
	IssueRowID   string
	LayerID      string
	Qty          types.Decimal
	Value        types.Decimal
	Currency     string
	VoucherType  string
	VoucherID    string
	Revision     int64
	GroupID      string
	Reversal     bool
	ReversesEdge string
}

func insertApplication(ctx context.Context, tx *sql.Tx, app application) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	reversal := 0
	if app.Reversal {
		reversal = 1
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO athena_item_applications (id, company_name, book, event, kind, issue_row_id, receipt_layer_id, applied_qty, applied_value, base_currency, voucher_type, voucher_id, voucher_revision, posting_group_id, is_reversal, reverses_application_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.String(), app.Company, app.Book, app.Event, app.Kind, app.IssueRowID, app.LayerID,
		app.Qty.String(), app.Value.String(), app.Currency,
		app.VoucherType, app.VoucherID, app.Revision, app.GroupID, reversal, nullString(app.ReversesEdge))
	return err
}

func insertLayer(ctx context.Context, tx *sql.Tx, m Movement, p PlannedMovement, rowID string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO athena_stock_layers (id, company_name, book, item_type, item_id, location_name, lot_no, receipt_row_id, base_uom, original_qty, residual_qty, unit_cost, base_currency, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open')`,
		id.String(), m.Company, m.Book, m.ItemType, m.ItemID, m.Location, nullString(m.LotNo),
		rowID, p.BaseUoM, p.BaseQty.String(), p.BaseQty.String(), p.UnitCost.String(), p.BaseCurrency)
	return err
}

func shiftResidual(ctx context.Context, tx *sql.Tx, layerID string, delta types.Decimal) error {
	_, err := tx.ExecContext(ctx, `UPDATE athena_stock_layers SET residual_qty = residual_qty + ?, status = CASE WHEN residual_qty + ? > 0 THEN 'open' ELSE 'consumed' END, revision = revision + 1 WHERE id = ?`,
		delta.String(), delta.String(), layerID)
	return err
}

// applyLayerOps writes the layer create, consumes and restores of a submit
// with their application edges. Touched layers are relocked and consumes
// rechecked, so Apply stays safe without a preceding Plan.
func applyLayerOps(ctx context.Context, tx *sql.Tx, rowID, groupID string, p PlannedMovement) error {
	m := p.Movement
	if p.create {
		if err := insertLayer(ctx, tx, m, p, rowID); err != nil {
			return err
		}
	}
	for _, op := range p.consume {
		layer, found, err := layerByID(ctx, tx, op.LayerID)
		if err != nil {
			return err
		}
		if !found || layer.Residual.Cmp(op.Qty) < 0 {
			return fmt.Errorf("%w: layer %s is short", ErrIntegrity, op.LayerID)
		}
		if err := shiftResidual(ctx, tx, op.LayerID, op.Qty.Neg()); err != nil {
			return err
		}
		if err := insertApplication(ctx, tx, application{
			Company: m.Company, Book: m.Book, Event: posting.EventSubmit, Kind: op.Kind,
			IssueRowID: rowID, LayerID: op.LayerID, Qty: op.Qty, Value: op.Value, Currency: p.BaseCurrency,
			VoucherType: m.VoucherType, VoucherID: m.VoucherID, Revision: m.Revision, GroupID: groupID,
		}); err != nil {
			return err
		}
	}
	for _, op := range p.restore {
		if _, _, err := layerByID(ctx, tx, op.LayerID); err != nil {
			return err
		}
		if err := shiftResidual(ctx, tx, op.LayerID, op.Qty); err != nil {
			return err
		}
		if err := insertApplication(ctx, tx, application{
			Company: m.Company, Book: m.Book, Event: posting.EventSubmit, Kind: op.Kind,
			IssueRowID: rowID, LayerID: op.LayerID, Qty: op.Qty, Value: op.Value, Currency: p.BaseCurrency,
			VoucherType: m.VoucherType, VoucherID: m.VoucherID, Revision: m.Revision, GroupID: groupID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// applyCancelLayers writes the layer shifts of one cancel mirror with
// flagged reversal edges.
func applyCancelLayers(ctx context.Context, tx *sql.Tx, p PlannedCancel, rowID, groupID string, mirror cancelMirror) error {
	orig := mirror.Orig
	shift := func(op layerOp, down bool) error {
		delta := op.Qty
		if down {
			delta = op.Qty.Neg()
		}
		if err := shiftResidual(ctx, tx, op.LayerID, delta); err != nil {
			return err
		}
		return insertApplication(ctx, tx, application{
			Company: orig.Bin.Company, Book: orig.Bin.Book, Event: posting.EventCancel, Kind: op.Kind,
			IssueRowID: rowID, LayerID: op.LayerID, Qty: op.Qty, Value: op.Value, Currency: orig.Currency,
			VoucherType: p.VoucherType, VoucherID: p.VoucherID, Revision: p.Revision, GroupID: groupID,
			Reversal: true, ReversesEdge: op.ReversesEdge,
		})
	}
	for _, op := range mirror.consume {
		if err := shift(op, true); err != nil {
			return err
		}
	}
	for _, op := range mirror.restore {
		if err := shift(op, false); err != nil {
			return err
		}
	}
	return nil
}
