import json
d = json.load(open('/mnt/nixstore/repose-ws/demo/session.json'))
L, styles, F = d['lines'], d['styles'], d['frames']
txt = lambda i: ''.join(s[0] for s in L[i])
def new_line(segs): L.append(segs); return len(L) - 1
# Typing plays in real time; once Claude is working, pauses are capped and sped up.
work = next(k for k, f in enumerate(F) if any('esc to interrupt' in txt(i) for i in f['l']))
# A repose guest's tmux has mouse and focus-events on (nix/guest/base/tmux.nix),
# so Claude Code's hint about them never shows there; the recording box lacked
# them. And a scrolled right pane can start on the tail of the old, longer path.
blank = new_line([['', 0]])
for f in F:
    f['l'] = [blank if 'tmux detected' in txt(i) else i for i in f['l']]
    if f['r'] and txt(f['r'][0]) == 'iting/apps/web': f['r'] = f['r'][1:]
seq, at = [], 0
for k, (a, b) in enumerate(zip(F, F[1:] + [None])):
    fr = {'at': at, 'l': a['l'], 'r': a['r']}
    if 'b' in a: fr['b'] = a['b']
    seq.append(fr)
    dt = ((b['t'] - a['t']) if b else 0) * 1000
    at += int(min(dt, 700)) if k < work else int(min(dt, 500) * 0.45)
at += 6500  # hold on the answer
det = [new_line([['[detached (from session izma)]', 0]]), new_line([['', 0]]),
       new_line([['~/code/recruiting on main', 0]]), new_line([['❯ ', 0]])]
seq.append({'at': at, 'full': det}); at += 2800
json.dump({'styles': styles, 'lines': L, 'seq': seq, 'T': at}, open('/home/azureuser/projects/factory/apps/web/src/lib/components/illustrations/session.json', 'w'), separators=(',', ':'))
print('frames', len(seq), 'work starts at frame', work, 'at', seq[work]['at'], 'ms; total', at, 'ms')
