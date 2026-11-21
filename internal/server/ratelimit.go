package server

import (
	"container/list"
	"context"
	"crypto/sha256"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/gen/athena/v1/athenav1connect"
	"athena-erp/internal/auth"
	"connectrpc.com/connect/v2"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
)

const bucketIdle = 15 * time.Minute // at least the longest full refill period

type bucketKey struct {
	category string
	digest   [32]byte
}
type rateBucket struct {
	key     bucketKey
	last    time.Time
	limiter *rate.Limiter
}
type rateBuckets struct {
	mu     sync.Mutex
	byKey  map[bucketKey]*list.Element
	recent list.List
	max    int
}

func newRateBuckets(max int) *rateBuckets {
	return &rateBuckets{max: max, byKey: make(map[bucketKey]*list.Element)}
}

// retry returns zero on admission. When full, preserve active budgets rather
// than letting new identities displace them and reset a depleted limit.
func (b *rateBuckets) retry(category, identity string, count int, period time.Duration, now time.Time) time.Duration {
	key := bucketKey{category, sha256.Sum256([]byte(identity))}
	b.mu.Lock()
	defer b.mu.Unlock()
	for oldest := b.recent.Back(); oldest != nil; oldest = b.recent.Back() {
		bucket := oldest.Value.(*rateBucket)
		if now.Sub(bucket.last) < bucketIdle {
			break
		}
		delete(b.byKey, bucket.key)
		b.recent.Remove(oldest)
	}
	el := b.byKey[key]
	limit := rate.Limit(float64(count) / period.Seconds())
	if el == nil {
		if b.recent.Len() >= b.max {
			return b.recent.Back().Value.(*rateBucket).last.Add(bucketIdle).Sub(now)
		}
		el = b.recent.PushFront(&rateBucket{key: key, limiter: rate.NewLimiter(limit, count)})
		b.byKey[key] = el
	}
	item := el.Value.(*rateBucket)
	item.last = now
	b.recent.MoveToFront(el)
	if item.limiter.Burst() != count {
		item.limiter.SetBurstAt(now, count)
		item.limiter.SetLimitAt(now, limit)
	}
	if item.limiter.AllowN(now, 1) {
		return 0
	}
	return max(time.Nanosecond, time.Duration(math.Ceil((1-item.limiter.TokensAt(now))/float64(limit)*float64(time.Second))))
}

type namedRequest interface {
	proto.Message
	GetUsername() string
}

func signInMessage(procedure string) namedRequest {
	switch procedure {
	case athenav1connect.AuthServiceLoginProcedure:
		return new(v1.LoginRequest)
	case athenav1connect.AuthServiceCompleteChallengeProcedure:
		return new(v1.CompleteChallengeRequest)
	case athenav1connect.AuthServiceStartResetProcedure:
		return new(v1.StartResetRequest)
	}
	return nil
}

// The username limit precedes even StartReset's permission gate. Read the
// unary message once and give the same decoded request to the normal handler.
type bufferedRequest struct {
	connect.ServerStream
	first proto.Message
}

func (s *bufferedRequest) Receive(msg any) error {
	if s.first == nil {
		return s.ServerStream.Receive(msg)
	}
	proto.Merge(msg.(proto.Message), s.first)
	s.first = nil
	return nil
}

func sessionRateClass(proc string) string {
	service, method, _ := strings.Cut(strings.TrimPrefix(proc, "/"), "/")
	switch {
	case strings.HasPrefix(method, "Export"):
		return auth.SessionExports
	case service == "athena.v1.ReportService" || strings.HasPrefix(method, "RunReport") || proc == athenav1connect.SystemServiceVerifyAuditLogProcedure:
		return auth.SessionReports
	case proc == athenav1connect.AuthServiceLoginProcedure || proc == athenav1connect.AuthServiceCompleteChallengeProcedure:
		return ""
	}
	for _, prefix := range []string{"Get", "List", "Lookup", "Preview", "Plan", "Explain", "Resolve", "Evaluate", "Simulate"} {
		if strings.HasPrefix(method, prefix) {
			return ""
		}
	}
	return auth.SessionWrites
}

func rateInterceptor(svc *auth.Service) connect.ServerInterceptor {
	buckets := newRateBuckets(8192)
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			info, ok := connect.CallInfoForServerContext(ctx)
			if svc == nil || !ok {
				return next(ctx, spec, stream)
			}
			signIn, class := signInMessage(spec.Procedure), sessionRateClass(spec.Procedure)
			if signIn == nil && class == "" {
				return next(ctx, spec, stream)
			}
			settings, err := svc.Security(ctx)
			if err != nil {
				return authError("read security settings", err)
			}
			check := func(category, identity string, period time.Duration) error {
				delay := buckets.retry(category, identity, settings[category], period, time.Now())
				if delay <= 0 {
					return nil
				}
				seconds := int(math.Ceil(delay.Seconds()))
				info.ResponseHeader().Set("Retry-After", strconv.Itoa(seconds))
				requestID, _ := requestFrom(ctx)
				svc.LogAccess(ctx, auth.AccessEvent{Event: auth.EventRateLimited, Outcome: auth.OutcomeDenied, Procedure: spec.Procedure, RequestID: requestID, Detail: category})
				return failureError(connect.CodeResourceExhausted, "too many requests; try again later", &v1.AuthFailure{Reason: "rate_limited", RetryAfterSeconds: uint32(seconds)})
			}
			if signIn != nil {
				ip, _, err := net.SplitHostPort(info.PeerAddr)
				if err != nil {
					ip = info.PeerAddr
				}
				if err := check(auth.SignInIP, ip, time.Minute); err != nil {
					return err
				}
				if err := stream.Receive(signIn); err != nil {
					return err
				}
				name, err := auth.NormalizeUsername(signIn.GetUsername())
				if err != nil {
					name = ""
				} // malformed names share one bounded bucket
				if err := check(auth.SignInUsername, name, 15*time.Minute); err != nil {
					return err
				}
				stream = &bufferedRequest{ServerStream: stream, first: signIn}
			}
			if class != "" {
				token := cookieValue(info.RequestHeader(), sessionCookieName)
				if token != "" && auth.CheckCSRF(token, info.RequestHeader().Get(csrfHeader)) {
					authenticate := svc.Authenticate
					if passiveProcedures[spec.Procedure] {
						authenticate = svc.AuthenticatePassive
					}
					p, err := authenticate(ctx, token)
					if err != nil {
						return authError("check session", err)
					}
					if err := check(class, p.RateID, time.Minute); err != nil {
						return err
					}
					ctx = auth.WithPrincipal(ctx, p)
				}
			}
			return next(ctx, spec, stream)
		}
	}
}
