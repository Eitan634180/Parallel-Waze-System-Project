import { createReadStream } from 'node:fs';
import { access, stat } from 'node:fs/promises';
import http from 'node:http';
import path from 'node:path';

const mimeTypes = {
  '.css': 'text/css; charset=utf-8',
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
};
const entryPath = '/public/index.html';

export async function startStaticServer(rootDir, host, port) {
  await access(rootDir);

  const server = http.createServer(async (req, res) => {
    try {
      if (shouldRedirectToEntry(req.url)) {
        sendRedirect(res, req.url || '/');
        return;
      }

      const requestPath = sanitizePath(req.url || '/');
      const filePath = path.join(rootDir, requestPath);
      const fileInfo = await stat(filePath);
      if (!fileInfo.isFile()) {
        sendNotFound(res);
        return;
      }

      const ext = path.extname(filePath);
      res.writeHead(200, { 'Content-Type': mimeTypes[ext] || 'application/octet-stream' });
      createReadStream(filePath).pipe(res);
    } catch {
      sendNotFound(res);
    }
  });

  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, host, resolve);
  });

  return server;
}

export function sanitizePath(rawUrl) {
  const rawPath = String(rawUrl || '/').split('?')[0].split('#')[0];
  const pathname = decodeURIComponent(rawPath.startsWith('/') ? rawPath : `/${rawPath}`);
  const pathSegments = pathname.split('/');
  if (pathSegments.includes('..')) {
    return entryPath.slice(1);
  }
  const normalized = path.posix.normalize(pathname).replace(/^\/+/, '');

  if (normalized.startsWith('..')) {
    return entryPath.slice(1);
  }
  return normalized || entryPath.slice(1);
}

function sendNotFound(res) {
  res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
  res.end('Not found');
}

function shouldRedirectToEntry(rawUrl) {
  const rawPath = String(rawUrl || '/').split('?')[0].split('#')[0];
  return rawPath === '/' || rawPath === '/index.html';
}

function sendRedirect(res, rawUrl) {
  const query = String(rawUrl || '').includes('?') ? `?${String(rawUrl).split('?')[1].split('#')[0]}` : '';
  res.writeHead(302, { Location: `${entryPath}${query}` });
  res.end();
}
