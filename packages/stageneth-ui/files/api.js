var api = async (path, method, body) => {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' }
  };
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch(path, opts);
  const text = await res.text().catch(() => '');
  let data;
  try { data = text ? JSON.parse(text) : {}; } catch (e) { data = null; }
  if (!data || typeof data !== 'object') {
    const preview = text.substring(0, 120).replace(/\n/g, ' ');
    throw new Error(`API ${path} status ${res.status}: non-JSON response: ${preview}`);
  }
  if (res.status === 401 && !path.endsWith('/session')) {
    // session expired or missing -> back to the login screen.
    // /session itself returns 401 for bad credentials — let the
    // caller show that error instead of silently reloading
    window.location.reload();
    return;
  }
  if (data.code !== 200) {
    throw new Error(`API ${path} status ${res.status}: ${data.message || JSON.stringify(data)}`);
  }
  return data;
};
