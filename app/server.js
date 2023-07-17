// server.js
import { createServer } from 'http';

const hostname = '127.0.0.1';
const port = 2112;

const server = createServer((req, res) => {
  res.statusCode = 200;
  res.setHeader('Content-Type', 'text/plain');
  res.end('Hello World! Welcome to Node.js');
});

server.listen(port, hostname, () => {
  console.log(`Server running at http://${hostname}:${port}/`);
});