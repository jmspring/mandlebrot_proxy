# Mandlebrot Auth Proxy

This project will be developed in four phases:
- initial proxy setup w/ logging -- assumes the mandlebrot container is already running
- add JWT support for authentication
- add programatic control of the mandlebrot proxy - leverage the Docker library for go (instead of shelling out)
- tying the above together.

This readme will include notes for each phase.  The most current instructions will be at the top.

# Running (phase 1)

Start the mandlebrot container manually:

```bash
docker run -p 8080:80 lechgu/mandelbrot
```

In the code directory (where this readme is) run:

```bash
go run .
# proxy is on :9090, forwarding to :8080
curl -X POST http://localhost:9090/generate/ \
  -H "Content-Type: application/json" \
  -d '{"width":640,"height":480,"iterations":100,"re_min":-2,"re_max":1,"im_min":-1,"im_max":1,"kind":"png"}' \
  -o mandelbrot.png
```

Tests can be run:

```bash
go test -v ./...
```