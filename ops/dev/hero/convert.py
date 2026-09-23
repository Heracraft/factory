import json, re
PATH_FROM = '/mnt/nixstore/repose-ws/demo/recruiting'
PATH_TO = '/home/dev/recruiting'
BASE16 = ['#1c1c1c','#e06c75','#98c379','#e5c07b','#61afef','#c678dd','#56b6c2','#dcdfe4',
          '#5c6370','#ef8f97','#b5e890','#f0d197','#8cc8ff','#dc9ef0','#7fd3de','#ffffff']
def xterm256(n):
    if n < 16: return BASE16[n]
    if n < 232:
        n -= 16; r, g, b = n // 36, (n // 6) % 6, n % 6
        c = lambda v: 0 if v == 0 else 55 + v * 40
        return '#%02x%02x%02x' % (c(r), c(g), c(b))
    v = 8 + (n - 232) * 10; return '#%02x%02x%02x' % (v, v, v)
SGR = re.compile(r'\x1b\[([0-9;:]*)m')
styles, style_ix, lines, line_ix = [], {}, [], {}
def sid(st):
    key = json.dumps(st, sort_keys=True)
    if key not in style_ix: style_ix[key] = len(styles); styles.append(st)
    return style_ix[key]
def parse_line(line):
    st = {}; segs = []; pos = 0
    for m in SGR.finditer(line):
        if m.start() > pos: segs.append([line[pos:m.start()], sid(dict(st))])
        pos = m.end()
        ps = [p for p in re.split('[;:]', m.group(1))] or ['0']
        i = 0
        while i < len(ps):
            p = int(ps[i] or 0)
            if p == 0: st = {}
            elif p == 1: st['b'] = 1
            elif p == 2: st['d'] = 1
            elif p == 3: st['i'] = 1
            elif p == 4: st['u'] = 1
            elif p == 7: st['r'] = 1
            elif p == 22: st.pop('b', None); st.pop('d', None)
            elif p == 23: st.pop('i', None)
            elif p == 24: st.pop('u', None)
            elif p == 27: st.pop('r', None)
            elif 30 <= p <= 37: st['fg'] = BASE16[p - 30]
            elif 90 <= p <= 97: st['fg'] = BASE16[p - 90 + 8]
            elif 40 <= p <= 47: st['bg'] = BASE16[p - 40]
            elif 100 <= p <= 107: st['bg'] = BASE16[p - 100 + 8]
            elif p == 39: st.pop('fg', None)
            elif p == 49: st.pop('bg', None)
            elif p in (38, 48):
                k = 'fg' if p == 38 else 'bg'
                if ps[i+1] == '5': st[k] = xterm256(int(ps[i+2])); i += 2
                elif ps[i+1] == '2': st[k] = '#%02x%02x%02x' % tuple(int(x) for x in ps[i+2:i+5]); i += 4
            i += 1
    if pos < len(line): segs.append([line[pos:], sid(dict(st))])
    # merge adjacent same-style
    out = []
    for text, s in segs:
        if out and out[-1][1] == s: out[-1][0] += text
        else: out.append([text, s])
    return out
def store(segs):
    key = json.dumps(segs)
    if key not in line_ix: line_ix[key] = len(lines); lines.append(segs)
    return line_ix[key]
def wrap(segs, width):
    rows, cur, n = [], [], 0
    for text, s in segs:
        while text:
            room = width - n
            if room == 0: rows.append(cur); cur, n = [], 0; continue
            take = text[:room]; cur.append([take, s]); n += len(take); text = text[room:]
    rows.append(cur)
    return rows
WIDTHS = (87, 62, 62)
frames = []
for raw in open('/mnt/nixstore/repose-ws/demo/frames.jsonl'):
    f = json.loads(raw)
    split = f['panes'][2] is not None
    limits = (36, 23 if split else 36, 12)
    panes = []
    for p, w, lim in zip(f['panes'], WIDTHS, limits):
        if p is None: panes.append(None); continue
        ids = []
        for l in p.rstrip('\n').split('\n'):
            for row in wrap(parse_line(l.replace(PATH_FROM, PATH_TO)), w): ids.append(store(row))
        panes.append(ids[:lim])
    fr = {'t': f['t'], 'l': panes[0], 'r': panes[1]}
    if split: fr['b'] = panes[2]
    frames.append(fr)
json.dump({'styles': styles, 'lines': lines, 'frames': frames}, open('/mnt/nixstore/repose-ws/demo/session.json', 'w'), separators=(',', ':'))
print(len(frames), 'frames', len(lines), 'unique lines', len(styles), 'styles', frames[-1]['t'], 's')
