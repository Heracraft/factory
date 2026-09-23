# typeit.py PANE "text" [enter] — types like a person: 60-170 ms per key,
# longer after spaces and punctuation, the odd pause to think.
import random, subprocess, sys, time
pane, text = sys.argv[1], sys.argv[2]
random.seed(7)
for ch in text:
    subprocess.run(['tmux', 'send-keys', '-t', pane, '-l', ch])
    d = random.uniform(0.04, 0.10)
    if ch == " ": d += random.uniform(0.02, 0.07)
    if ch in '.,': d += random.uniform(0.15, 0.35)
    if random.random() < 0.03: d += random.uniform(0.3, 0.7)
    time.sleep(d)
if len(sys.argv) > 3:
    time.sleep(0.35); subprocess.run(['tmux', 'send-keys', '-t', pane, 'Enter'])
