import re, sys

def ident(tok):
    # compound identifier containing Record/record
    tok = re.sub(r'Record(s?)(?=[A-Z]|$)', r'RecordType\1', tok)
    tok = re.sub(r'(?<![A-Za-z])record(?=[A-Z])', 'recordType', tok)
    return tok

def code_seg(s):
    s = re.sub(r'athenav1\.Record(?![A-Za-z])', 'athenav1.RecordType', s)
    s = re.sub(r'athenav1\.Records(?![A-Za-z])', 'athenav1.RecordTypes', s)
    s = re.sub(r'(?<=\.)Records\b', 'RecordTypes', s)
    s = re.sub(r'(?<=\.)Record\b', 'RecordType', s)
    s = re.sub(r'(?<![\w.])Record(?=:)', 'RecordType', s)
    s = re.sub(r'^(\s*)Record(?= string)', r'\1RecordType', s)
    s = re.sub(r'\b[A-Za-z_]\w*\b', lambda m: ident(m.group(0)) if m.group(0) not in ('Record','Records') else m.group(0), s)
    s = re.sub(r'\brecords\b', 'recordTypes', s)
    s = re.sub(r'\brecord\b', 'recordType', s)
    return s

def prose_seg(s):
    s = re.sub(r'\b[A-Za-z_]\w*\b', lambda m: ident(m.group(0)) if m.group(0) not in ('Record','Records') else m.group(0), s)
    s = re.sub(r'\bRecords\b', 'Record Types', s)
    s = re.sub(r'\bRecord\b', 'Record Type', s)
    s = re.sub(r'\brecords\b', 'record types', s)
    s = re.sub(r'\brecord\b', 'record type', s)
    return s

def process(text):
    out=[]; raw=False
    for line in text.split('\n'):
        i=0; segs=[]; cur=''; mode='raw' if raw else 'code'
        if raw:
            pass
        n=len(line)
        buf=''
        while i<n:
            c=line[i]
            if mode=='code':
                if line.startswith('//',i):
                    segs.append(('code',buf)); segs.append(('prose',line[i:])); buf=''; i=n; break
                if c=='"':
                    segs.append(('code',buf)); buf='"'; mode='str'; i+=1; continue
                if c=='`':
                    segs.append(('code',buf)); buf='`'; mode='raw'; i+=1; continue
                buf+=c; i+=1
            elif mode=='str':
                buf+=c
                if c=='\\' and i+1<n: buf+=line[i+1]; i+=2; continue
                i+=1
                if c=='"': segs.append(('prose',buf)); buf=''; mode='code'
            else:
                buf+=c; i+=1
                if c=='`': segs.append(('prose',buf)); buf=''; mode='code'
        if buf:
            segs.append(('prose' if mode in('str','raw') else 'code', buf))
        raw = (mode=='raw')
        out.append(''.join((code_seg(s) if k=='code' else prose_seg(s)) for k,s in segs))
    return '\n'.join(out)

pre=[('recordHistory','writeHistory'),('athena_record_operations','athena_record_type_operations'),('athena_records','athena_record_types'),
('athena_entry_history','athena_record_history'),('record_name','record_type_name'),('entry_id','record_id'),('target_record','target_record_type'),('fk_record_','fk_rtype_')]
post=[('Entries','Records'),('entries','records'),('Entry','Record'),('entry','record')]
for path in sys.argv[1:]:
    t=open(path).read()
    for a,b in pre: t=t.replace(a,b)
    t=process(t)
    for a,b in post: t=t.replace(a,b)
    open(path,'w').write(t)
