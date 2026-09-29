import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { readResource } from '../src/browser/transport.js';
// Only DNS validation is replaced: all requests below use real Node HTTP sockets
// to a controlled loopback fixture while retaining the public URL's Host header.
vi.mock('../src/browser/security.js', () => ({
  validateUrl: async (input: string) => ({
    url: new URL(input),
    address: '127.0.0.1',
    family: 4,
  }),
}));
let server: Server;
let base: string;
let privateHits: number;
beforeEach(async () => {
  privateHits = 0;
  server = createServer((request, response) => {
    if (request.url === '/redirect') {
      response.writeHead(302, { Location: `${base}/private` });
      response.end();
    } else if (request.url === '/private') {
      privateHits++;
      response.end('secret');
    } else if (request.url === '/slow') {
      request.on('close', () => response.destroy());
    } else {
      response.writeHead(200, { 'Content-Type': 'text/html' });
      response.end(`<h1>Public fixture</h1><p>${request.headers.host}</p>`);
    }
  });
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  base = `http://public.example:${(server.address() as AddressInfo).port}`;
});
afterEach(async () => {
  server.closeAllConnections();
  await new Promise<void>((resolve) => server.close(() => resolve()));
});
describe('DNS-pinned transport', () => {
  it('supports Node 24 lookup all mode on an actual HTTP connection', async () => {
    const response = await readResource(base);
    expect(response.status).toBe(200);
    expect(response.body.toString()).toContain('Public fixture');
    expect(response.body.toString()).toContain('public.example');
  });
  it('rejects redirects before returning them to a browser or contacting the destination', async () => {
    await expect(readResource(`${base}/redirect`)).rejects.toThrow(
      'Redirects are blocked',
    );
    expect(privateHits).toBe(0);
  });
  it('aborts in-flight transport when the browser client disconnects', async () => {
    const controller = new AbortController();
    const pending = readResource(`${base}/slow`, controller.signal);
    const rejected = expect(pending).rejects.toThrow();
    controller.abort();
    await rejected;
  });
});
