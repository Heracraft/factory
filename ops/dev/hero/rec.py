import subprocess, time, json, sys
out = open('/mnt/nixstore/repose-ws/demo/frames.jsonl', 'w')
t0 = time.time(); last = None
def cap(i):
    r = subprocess.run(['tmux','capture-pane','-p','-e','-J','-t',f'rec:0.{i}'], capture_output=True, text=True)
    return r.stdout if r.returncode == 0 else None
while time.time() - t0 < float(sys.argv[1]):
    panes = [cap(i) for i in (0, 1, 2)]
    if panes != last:
        out.write(json.dumps({'t': round(time.time()-t0, 3), 'panes': panes}) + '\n'); out.flush(); last = panes
    time.sleep(0.2)
