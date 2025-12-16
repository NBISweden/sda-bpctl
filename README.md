# sda-bpctl

A tool that can be used to deal with administrative workflows for the big picture project. It supports three primary functions, making data ingestion, assigning accession ids to each ingested file, and creating a dataset for all files ingested with a accession id.

It can be used either locally as a cli tool or be packaged and run as a job in kubernetes.

### installation

Build from source using `go`:

```bash
git clone git@github.com:NBISweden/sda-bpctl.git
cd sda-bpctl
go build -o bpctl .
bpctl -h
```

### usage

The CLI have one requiered argument, called a **command** and non-requiered input arguments as flags. The rest of configuration is done through a config file. See more in the configuration section.

Commands must be one of:

- `ingest`
- `accession`
- `dataset`
- `mail`
- `job`

example:
```bash
./bpctl ingest
```

### configuration

bpctl can consume configuration from either `config.yaml` or from environment variables. If both are supplied then the environment variables will take priority. If using config.yaml it is expected to be located in the root directory of the project

see the `config.yaml.example` or `job.yaml.example` for a base template with what fields to fill

### contribute

As of right now there are no explicit rules. Feel free to reach out if you have any questions `erik.zeidlitz@nbis.se`

### testing

Unit tests using [pkg.go.dev/testing](https://pkg.go.dev/testing) 

Running all tests:
```bash
go test ./...
