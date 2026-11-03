const qs = new URLSearchParams(location.search);
  const target = qs.get('target') || '';
  const reason = qs.get('reason') || '未知错误';
  const source = qs.get('source') || '';
  const $ = (id) => document.getElementById(id);
  $('reason').textContent = source ? `${reason}（当前地址来自${source}）` : reason;
  $('target').textContent = target || '—';

  const invoke = (cmd, args) => {
    const ipc = window.__TAURI__ && window.__TAURI__.core;
    if (!ipc) return Promise.reject(new Error('壳内 IPC 不可用'));
    return ipc.invoke(cmd, args);
  };

  let busy = false;
  const setBusy = (v, text) => { busy = v; $('retry').disabled = v; $('save').disabled = v; $('hint').textContent = text || ''; };

  $('retry').addEventListener('click', () => {
    if (busy) return;               // 连点保护：并发导航以最后一次生效，先挡掉重复提交
    setBusy(true, '正在重新连接…');
    invoke('probe_server', { url: target })
      .then(() => setBusy(true, ''))
      .catch((e) => setBusy(false, String(e && e.message ? e.message : e)));
  });

  $('edit').addEventListener('click', () => {
    $('form').style.display = 'block';
    $('url').value = target;
    $('url').focus();
    $('hint').textContent = '';
  });

  $('cancel').addEventListener('click', () => { $('form').style.display = 'none'; $('hint').textContent = ''; });

  $('save').addEventListener('click', () => {
    if (busy) return;
    const value = $('url').value.trim();
    if (!value) { $('hint').textContent = '地址不能为空'; return; }
    setBusy(true, '正在连接…');
    invoke('set_server_url', { url: value })
      .then(() => setBusy(true, ''))
      .catch((e) => setBusy(false, String(e && e.message ? e.message : e)));
  });
