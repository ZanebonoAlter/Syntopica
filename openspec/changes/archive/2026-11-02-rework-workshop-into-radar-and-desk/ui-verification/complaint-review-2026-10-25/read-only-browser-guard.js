(() => {
  if (window.__complaintReadOnlyGuard) return;
  window.__complaintReadOnlyGuard = true;
  window.__complaintBlockedWrites = [];
  const allowed = method => ['GET', 'HEAD', 'OPTIONS'].includes(String(method || 'GET').toUpperCase());
  const blocked = (method, url) => {
    window.__complaintBlockedWrites.push({ method, url: String(url) });
    return new Error('Read-only review: non-read request blocked');
  };
  const originalFetch = window.fetch.bind(window);
  window.fetch = (input, init) => {
    const method = init?.method || (input instanceof Request ? input.method : 'GET');
    if (!allowed(method)) return Promise.reject(blocked(method, input instanceof Request ? input.url : input));
    return originalFetch(input, init);
  };
  const originalOpen = XMLHttpRequest.prototype.open;
  const originalSend = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function(method, url, ...args) {
    this.__reviewMethod = method;
    this.__reviewUrl = url;
    return originalOpen.call(this, method, url, ...args);
  };
  XMLHttpRequest.prototype.send = function(...args) {
    if (!allowed(this.__reviewMethod)) throw blocked(this.__reviewMethod, this.__reviewUrl);
    return originalSend.apply(this, args);
  };
  navigator.sendBeacon = url => { blocked('BEACON', url); return false; };
})();
