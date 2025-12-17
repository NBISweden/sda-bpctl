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

#### examples

some examples to demonstrate how the tool can be used

running ingest 
```bash
./bpctl ingest
```

running accession with a specific config.yaml
```bash
./bpctl accession --config /home/config.yaml
```

running mail notification with the dry-run flag
```bash
./bpctl mail --dry-run
```

#### kubernetes job

The `job` command is meant to try and run all the steps of the dataset submission process in order; ingest -> accession -> dataset. It will need environment variables specified as found in `job.yaml.example` and will need to be adjusted for the environment to run in. The `job` command also needs a input argument that represents the ammount of files that is expected to be included in the finalized dataset. If at some point during the process this number does not match the job will fail and the user have to take over the process from that point.

specify your maniifest, for example in a `job.yaml` and it can be started using `kubectl`

```bash
kubectl apply -f job.yaml
```

### configuration

bpctl can consume configuration from either `config.yaml` or from environment variables. If both are supplied then the environment variables will take priority. If using config.yaml it is expected to be located in the root directory of the project. It can also be supplied by using the `--config` flag if located elsewhere.

see the `config.yaml.example` or `job.yaml.example` for a base template with what fields to fill

### testing

Unit tests using [pkg.go.dev/testing](https://pkg.go.dev/testing) 

Running all tests:
```bash
go test ./...
```
