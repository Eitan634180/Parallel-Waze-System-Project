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

export async function startStaticServer(rootDir, port) {
  await access(rootDir);

  const server = http.createServer(async (req, res) => {
    try {
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
    server.listen(port, '127.0.0.1', resolve);
  });

  return server;
}

export function sanitizePath(rawUrl) {
  const rawPath = String(rawUrl || '/').split('?')[0].split('#')[0];
  const pathname = decodeURIComponent(rawPath.startsWith('/') ? rawPath : `/${rawPath}`);
  const pathSegments = pathname.split('/');
  if (pathSegments.includes('..')) {
    return 'navigation.html';
  }
  const normalized = path.posix.normalize(pathname).replace(/^\/+/, '');

  if (normalized.startsWith('..')) {
    return 'navigation.html';
  }
  return normalized || 'navigation.html';
}

function sendNotFound(res) {
  res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
  res.end('Not found');
}
