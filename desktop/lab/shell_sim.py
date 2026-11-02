#!/usr/bin/env python3
"""shell_sim.py — 0-3 sidecar 进程协议模拟壳（模拟 Tauri 的生命周期行为）.

场景：
  ready   — spawn server → 轮询 /health 握手 → 量「启动到 ready」时序
  stop    — SIGTERM → 等退出 → 量「关停命令到进程消失」时序 + 抓四步关停汇总行
  orphan  — 壳先死 → 观察 sidecar 是否残留（ppid watch 未实现前的现状基线）

用法：python3 shell_sim.py <ready|stop|orphan>
前提：线上后端在跑（本脚本随机端口避让；server 短暂双实例，窗口秒级）。
副作用说明：server 启动会跑一次幂等的孤儿进展收敛 sweep（既有行为，无害）。
"""
import json, os, random, signal, socket, subprocess, sys, time, urllib.request

BIN = os.path.expanduser('~/.syntopica/bench/server-bin')  # go build ./cmd/server 产物（验证前现编）
RUN = os.path.expanduser('~/software/Syntopica/desktop/lab/run')
os.makedirs(RUN, exist_ok=True)

def free_port():
    s = socket.socket(); s.bind(('127.0.0.1', 0)); p = s.getsockname()[1]; s.close(); return p

def spawn(port, tag):
    log = open(f'{RUN}/{tag}.log', 'w')
    env = dict(os.environ, SERVER_PORT=str(port))
    p = subprocess.Popen([BIN], cwd=RUN, env=env, stdout=log, stderr=subprocess.STDOUT,
                         start_new_session=True)  # 独立会话≈sidecar 语义（壳死后不随组信号死）
    return p, log

def wait_health(port, timeout=60):
    t0 = time.time()
    while time.time() - t0 < timeout:
        try:
            with urllib.request.urlopen(f'http://127.0.0.1:{port}/health', timeout=1) as r:
                if r.status == 200:
                    return time.time() - t0
        except Exception:
            time.sleep(0.1)
    return None

def alive(pid): 
    try: os.kill(pid, 0); return True
    except OSError: return False

def scene_ready():
    port = free_port(); t0 = time.time()
    p, log = spawn(port, 'ready')
    dt = wait_health(port)
    ms = (time.time()-t0)*1000
    if dt is None:
        print(f'READY FAIL: 60s 未就绪，日志尾部：'); log.flush(); print(open(f'{RUN}/ready.log').read()[-400:]); p.kill(); sys.exit(1)
    print(f'✅ 场景ready: spawn→/health 200 = {ms:.0f} ms（端口 {port}，含进程启动+DB 连接+路由装配）')
    os.kill(p.pid, signal.SIGTERM); p.wait(timeout=60); log.close()
    print('   （已随手指发 SIGTERM 清理）')

def scene_stop():
    port = free_port()
    p, log = spawn(port, 'stop')
    if wait_health(port) is None: print('起不来'); sys.exit(1)
    time.sleep(0.5)  # 稳定态
    t0 = time.time(); os.kill(p.pid, signal.SIGTERM)
    p.wait(timeout=120); dt = (time.time()-t0)*1000
    log.close()
    summary = [l for l in open(f'{RUN}/stop.log') if 'hutdown' in l and ('http' in l or 'registry' in l or 'workers' in l or 'summary' in l.lower())]
    print(f'✅ 场景stop: SIGTERM→进程退出 = {dt:.0f} ms')
    for l in summary[-4:]: print(f'   {l.strip()[:150]}')

def scene_orphan():
    port = free_port()
    p, log = spawn(port, 'orphan')
    if wait_health(port) is None: print('起不来'); sys.exit(1)
    pid = p.pid; log.close()
    # 模拟壳被爆头（SIGKILL 自己，sidecar 无感知）
    os.kill(os.getpid(), signal.SIGKILL)  # 到不了下一行——由外部观察
    _ = pid

if __name__ == '__main__':
    sc = sys.argv[1] if len(sys.argv) > 1 else 'ready'
    {'ready': scene_ready, 'stop': scene_stop, 'orphan': scene_orphan}[sc]()
