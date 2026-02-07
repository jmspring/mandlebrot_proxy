# Mandlebrot Auth Proxy

This project will be developed in four phases:
- initial proxy setup w/ logging -- assumes the mandlebrot container is already running
- add JWT support for authentication
- add programatic control of the mandlebrot proxy - leverage the Docker library for go (instead of shelling out)
- tying the above together.

This readme will include notes for each phase.  The most current instructions will be at the top.  Three and four will go together since technically docker implementation is separate from network/proxy, but proxy code already written.

It was chosen to use the go Docker sdk rather than shelling to keep better control over / clean up of the process management.  

This code was tested on Archi Linux with Docker and MacOS w/ OrbStack (a Docker drop in).

# Running

There is now a Makefile.  Running the project is outlined below.  Note, per the original document, the mandlebrot docker image returns a 317 when there isn't a trailing slash on the url for `generate/`.  So, that has been added here.

The instructions to run - 

```
go mod tidy
make run
```

`go mod tidy` syncs go.mod and go.sum.

Note that the proxy generates a dev token that can be used.  That said, it's also easy enough with just the token endpoint to generate such.

Grab a token:

```bash
TOKEN=$(curl -s -X POST http://localhost:9090/token \
  -H "Content-Type: application/json" \
  -d '{"subject":"demo","duration":"1h"}' | jq -r .token)
```

Request the mandlebrot image:

```
curl -X POST http://localhost:9090/generate/ \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "width": 640, "height": 480, "iterations": 100,
    "re_min": -2, "re_max": 1, "im_min": -1, "im_max": 1,
    "kind": "png"
  }' -o mandelbrot.png
```

To show authenticationw working, the following will generate a 401 - 

```bash
curl -s http://localhost:9090/generate
```

## Config

It is possible to set environment variables, those options are:

`LISTEN_ADDR` - default: `:9090`
`MANDELBROT_IMAGE` - default: `lechgu/mandelbrot`
`CONTAINER_PORT` - default: `8080`
`JWT_SECRET` - default - dev default - if this was production, probably should be a real value
`LOG_LEVEL` - default - `info` --> uses standard slog levels (debug, error, etc)

## Running Tests

`make test` - runs unit and integration tests *NOT* including docker related tests
`make test-all` - the above plus docker
`make cover` - test coverage report

## Other Makefile stuff

`make lint`- run the linter
`make clean` - clean things up

-------------------------
OLD 


# Running (phase 2)

Start the mandlebrot container manually:

```bash
docker run -p 8080:80 lechgu/mandelbrot
```

In the code directory (where this readme is) run:

```bash
go run .
# grab the dev token from the startup log, or:
TOKEN=$(curl -s -X POST http://localhost:9090/token \
  -H "Content-Type: application/json" \
  -d '{"subject":"demo","duration":"1h"}' | jq -r .token)

curl -X POST http://localhost:9090/generate/ \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"width":640,"height":480,"iterations":100,"re_min":-2,"re_max":1,"im_min":-1,"im_max":1,"kind":"png"}' \
  -o mandelbrot.png

# without auth:
curl -s http://localhost:9090/generate/
# {"error":"missing Authorization header"}
```



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