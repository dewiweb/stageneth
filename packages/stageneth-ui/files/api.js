var api = async (path, method, body) => {
  const token = localStorage.getItem('stageneth_token_v2');
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' }
  };
  if (token) opts.headers.Authorization = `Bearer ${token}`;
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch(path, opts);
  const text = await res.text().catch(() => '');
  let data;
  try { data = text ? JSON.parse(text) : {}; } catch (e) { data = null; }
  if (!data || typeof data !== 'object') {
    const preview = text.substring(0, 120).replace(/\n/g, ' ');
    throw new Error(`API ${path} status ${res.status}: non-JSON response: ${preview}`);
  }
  if (data.code !== 200) {
    throw new Error(`API ${path} status ${res.status}: ${data.message || JSON.stringify(data)}`);
  }
  if (res.status >= 400) {
    if (res.status === 403 || (data.message && data.message.toLowerCase().includes('invalid token'))) {
      localStorage.removeItem('stageneth_token_v2');
      window.location.reload();
      return;
    }
    throw new Error(data.message || res.statusText);
  }
  return data;
};
