import http.server
import socketserver

class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        self.send_response(200)
        self.send_header("Content-type", "application/json")
        self.end_headers()
        content_length = int(self.headers["Content-Length"]) if "Content-Length" in self.headers else 0
        post_data = self.rfile.read(content_length) if content_length > 0 else b""
        print(f"Received callback: {post_data.decode()}")
        self.wfile.write(b'{"status": "accepted"}')

if __name__ == "__main__":
    with socketserver.TCPServer(("0.0.0.0", 8080), Handler) as httpd:
        print("Mock callback server running at http://0.0.0.0:8080")
        httpd.serve_forever()