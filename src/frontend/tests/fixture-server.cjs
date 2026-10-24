// UI test fixture only. Not bundled or used by the desktop application.
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const root = path.resolve(__dirname, '../dist')
const mime = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2', '.woff': 'font/woff' }
http.createServer((req, res) => {
  const name = decodeURIComponent(new URL(req.url, 'http://localhost').pathname)
  if (name === '/qa-fixtures.js') { res.setHeader('Content-Type', mime['.js']); res.end(fs.readFileSync(path.join(__dirname, 'ui-fixtures.js'))); return }
  const file = path.resolve(root, name === '/' ? 'index.html' : `.${name}`)
  if (!file.startsWith(root + path.sep)) { res.writeHead(403); res.end(); return }
  try {
    let contents = fs.readFileSync(file)
    if (name === '/') contents = contents.toString().replace('</head>', '<script src="/qa-fixtures.js"></script></head>')
    res.setHeader('Content-Type', mime[path.extname(file)] || 'application/octet-stream'); res.end(contents)
  } catch { res.writeHead(404); res.end() }
}).listen(4173, '127.0.0.1', () => console.log('Synthetic UI fixtures at http://127.0.0.1:4173'))
