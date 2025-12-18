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

The `job` command is meant to try and run all the steps of the dataset submission process in order; ingest -> accession -> dataset. It will need environment variables to configure and will need to be adjusted for the environment to run in. The `job` command also needs a input argument that represents the ammount of files that is expected to be included in the finalized dataset. If at some point during the process this number does not match the job will fail and the user have to take over the process from that point.

specify your maniifest, for example in a `job.yaml` or you can render a templated manifest based on your `config.yaml` using the `render` command

```bash
./bpctl render -o job.yaml
```

and apply it using `kubectl`:

```bash
kubectl apply -f job.yaml
```


will render a job.yaml manifest for you based on the configuration values you have supplied


### configuration

bpctl can consume configuration from either `config.yaml` or from environment variables. If both are supplied then the environment variables will take priority. If using config.yaml it is expected to be located in the root directory of the project. It can also be supplied by using the `--config` flag if located elsewhere.

see the `config.yaml.example` for a base template with what fields to fill
| Name | Example | Description | used by |
| --------------- | --------------- | --------------- | --------------- |
| USER_ID | "user-1234" | The user ID for the uploader, acts as identifier for the uploaded data | `ingest`, `accession`, `dataset`, `job`, `render` |
| DATASET_ID | "aa-Dataset-abc" | The ID that will be set for the finalized dataset, will be used during the `dataset` command | `ingest`, `accession`, `dataset`, `job`, `mail`, `render` |
| DATASET_FOLDER | "DATASET_ABC" | The folder where the uploaded data resides in s3inbox | `ingest`, `accession`, `dataset`, `job`, `mail`, `render` |
| JOB_EXPECTED_NR_FILES | 0 | The expected number of files to be part of the finalized dataset, set this when using `render` to include it in the rendered job.yaml | `job`, `render` |
| CLIENT_API_HOST | "https://api.example.com" | The hostname for the SDA API to communicate with | `ingest`, `accession`, `dataset`, `job` |
| CLIENT_ACCESS_TOKEN | "youraccesstoken" | The access token to authenticate towards the client api host | Yes | `ingest`, `accession`, `dataset`, `job` |
| MAIL_ADDRESS | "myemail@example.com" | Used for the `mail` command, this will be the email address the outgoing emails will be sent from | `mail` |
| MAIL_PASSWORD | "mypasswordemail" | Password associated with mail address | `mail` |
| MAIL_UPLOADER | "jane@example.com" | Mail address to the uploader, this is the address the outgoing email will be sent to | `mail` |
| MAIL_UPLOADER_NAME | "Jane Doe" | Name of the uploader | `mail` |
| MAIL_SMTP_HOST | "smtp.example.com" | Hostname to a mail server to relay mails through | `mail` |
| MAIL_SMTP_PORT | 587 | Port for the mail server | `mail` |
| DB_SECRET_NAME | "db-secret" | The name of the kubernetes secret that holds connection details for the sda database | `job` ,`render` |
| CERT_SECRET_NAME | "cert-secret" | The name of the kubernetes secret that holds a tls certificate to use | `job`, `render` |

### testing

Unit tests using [pkg.go.dev/testing](https://pkg.go.dev/testing) 

Running all tests:
```bash
go test ./...
```
