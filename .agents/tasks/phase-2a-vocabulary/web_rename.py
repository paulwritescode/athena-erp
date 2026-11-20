import re, sys

def prose_bare(text):
    text = re.sub(r'\bRecord(s?)\b(?![<(])', lambda m: 'Record Types' if m.group(1) else 'Record Type', text)
    text = re.sub(r'(?<![\w-])record(s?)(?![\w-])', lambda m: 'record types' if m.group(1) else 'record type', text)
    return text

def stage_a(t, prose=True):
    t = t.replace('Record<', '@@G@@<')
    t = t.replace('target_record', 'target_record_type')
    t = t.replace('RecordDefinition', 'RecordType')
    # identifiers: camelCase tokens containing Record/record
    def ident(m):
        tok = m.group(0)
        if tok in ('Record', 'Records', 'RecordType', 'RecordTypes') or 'RecordType' in tok or 'recordType' in tok:
            return tok
        tok = re.sub(r'Record(s?)(?=[A-Z]|$)', r'RecordType\1', tok)
        tok = re.sub(r'(?<![A-Za-z])record(?=[A-Z])', 'recordType', tok)
        return tok
    t = re.sub(r'[A-Za-z_]\w*', ident, t)
    # bare lowercase code words (not CSS classes such as record-editor)
    t = re.sub(r'(?<![\w-])records(?![\w-])', 'recordTypes', t)
    t = re.sub(r'(?<![\w-])record(?![\w-])', 'recordType', t)
    if prose:
        t = re.sub(r'\bRecord(s?)\b(?![<(\w])', lambda m: 'Record Types' if m.group(1) else 'Record Type', t)
    return t

def stage_b(t):
    # protect CSS classes, element ids and the storage key
    for a, b in [('entries-', '@@ES@@'), ('entry-', '@@EY@@'), ('Entries-', '@@CES@@')]:
        t = t.replace(a, b)
    t = re.sub(r'\bEntry\b', 'RecordMessage', t)
    t = t.replace('EntryEvent', 'RecordEvent')
    for a, b in [('Entries', 'Records'), ('entries', 'records'), ('Entry', 'Record'), ('entry', 'record')]:
        t = t.replace(a, b)
    for a, b in [('@@ES@@', 'entries-'), ('@@EY@@', 'entry-'), ('@@CES@@', 'Entries-')]:
        t = t.replace(a, b)
    t = t.replace('@@G@@<', 'Record<')
    return t

def fix_imports(t):
    out = []
    for line in t.split('\n'):
        if line.startswith('import') and 'records_pb' in line:
            line = re.sub(r'type RecordMessage', 'type Record as RecordMessage', line)
        out.append(line)
    return '\n'.join(out)

if __name__ == '__main__':
    mode = sys.argv[1]
    for path in sys.argv[2:]:
        t = open(path).read()
        t = stage_a(t)
        if mode == 'both':
            t = stage_b(t)
            t = fix_imports(t)
        else:
            t = t.replace('@@G@@<', 'Record<')
            for a, b in [('Entries', 'Records'), ('entries', 'records'), ('Entry', 'Record'), ('entry', 'record')]:
                t = t.replace(a, b)
        open(path, 'w').write(t)
