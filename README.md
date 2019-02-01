Compiling
=========

Requires Go >= 1.11:

    export GO111MODULE=on
    go build

Running
=======

    ./osmosis

CA certificate will be generated automatically.

Import CA
=========

Configure proxy (default: `http://localhost:8080`), visit `http://proxy/ca` and import CA certificate.
