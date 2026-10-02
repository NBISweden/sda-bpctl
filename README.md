# Sensitive Data Archive - Big Picture Control (sda-bpctl)

A tool that can be used to deal with administrative workflows for the big picture project. It supports three primary functions, making data ingestion, assigning accession ids to each ingested file, and creating a dataset for all files ingested with a accession id.

bpctl can be used locally as a cli tool or it can be packaged and run as a job in kubernetes. 

The core functionallity of this tool is wrapping logic around the sensitive data archive (SDA) api, usually just referenced as 'the API' in this project. To fully understand how this is expected to work you should be familiar with the SDA and its api.

### installation

Build from source:

```bash
git clone git@github.com:NBISweden/sda-bpctl.git
cd sda-bpctl
go build -o bpctl .
./bpctl -h
```

### usage

The CLI have one required argument, called a **command** and non-required input arguments as flags. The rest of configuration is done through a config file. See more in the configuration section.

Commands must be one of:

- `ingest`
- `accession`
- `dataset`
- `mail`
- `job`
- `render`
- `status`

#### examples

Some examples to demonstrate how the tool can be used

Running ingest 
```bash
./bpctl ingest
```

Running accession with a specific config.yaml
```bash
./bpctl accession --config /home/config.yaml
```

Running mail notification with the dry-run flag
```bash
./bpctl mail --dry-run
```

#### kubernetes job

The `job` command is meant to try and run all the steps of the dataset submission process in order; ingest -> accession -> dataset. It will need environment variables to configure and will need to be adjusted for the environment to run in. The `job` command also needs a input argument that represents the amount of files that is expected to be included in the finalized dataset. If at some point during the process this number does not match the job will fail and the user have to take over the process from that point.

specify your manifest, for example in a `job.yaml` or you can render a templated manifest based on your `config.yaml` using the `render` command

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

see the `config.yaml.example` for a base template with what fields to fill. The example shows a minimal config. The table below shows all possible values that can be configured:
| Name                              | Default                           |
| --------------------------------- | --------------------------------- |
| DATASET_FOLDER                    | none                              |
| DATASET_ID                        | none                              | 
| USER_ID                           | none                              |
| SSL_CA_CERT                       | none                              |
| JOB_TIMEOUT                       | 4320                              |
| JOB_POLL_RATE                     | 180                               |
| JOB_DATA_DIRECTORY                | "/data"                           |
| CLIENT_API_HOST                   | "https://api.bp.nbis.se"          |
| CLIENT_ACCESS_TOKEN               | "sda-bpctl-mail"                  |
| CERT_SECRET_NAME                  | "sda-sda-svc-api-certs"           |
| STORAGE_SECRET_NAME               | "sda-bpctl-storage"               |
| MAIL_SECRET_NAME                  | none                              |
| MAIL_ADDRESS                      | none                              |
| MAIL_PASSWORD                     | none                              |
| MAIL_SMTP_HOST                    | "mail.nbis.se"                    |
| MAIL_SMTP_PORT                    | 587                               |
| MAIL_UPLOADER_NAME                | none                              |
| MAIL_UPLOADER_ORGANIZATION_NAME   | none                              |
| MAIL_UPLOADER                     | none                              |
| S3_INBOX_ENDPOINT                 | "s3a4.sto2.safedc.net"            |
| S3_INBOX_BUCKET                   | "inbox-2024-01"                   |
| S3_INBOX_ACCESS_KEY               | none                              |
| S3_INBOX_SECRET_KEY               | none                              |
| S3_METADATA_ENDPOINT              | "storage.sto3.safedc.net"         |
| S3_METADATA_BUCKET                | "public-metadata"                 |
| S3_METADATA_ACCESS_KEY            | none                              |
| S3_METADATA_SECRET_KEY            | none                              |
| C4GH_SEC_PEM                      | none                              |
| C4GH_PASSPHRASE                   | none                              |

### ingest

```bash
./bpctl ingest [flags]
```

The ingest command will lookup all the files for the `USER_ID` that resides in `DATASET_FOLDER`, filter out all files that are not in either a directory `LANDING_PAGE` or `PRIVATE` and any file that does not have the event `uploaded`. 

Files sent to ingestion are done so trough the sda api `POST /ingest` endpoint.

### accession

```bash
./bpctl accession [flags]
```

The accession command will get a list of files for the `USER_ID` that resides in `DATASET_FOLDER` and have the event `verified`.

Will create a file called `<DATASET_FOLDER>-fileIDs.txt` in the `--data-directory` directory. It will retrieve the list of files and after successful call to `POST /accession` it will write the accessionIDs to the file `<DATASET_FOLDER>-fileIDs.txt`. This is legacy logic owned from the `ingestor.sh` script and makes it so that you can store a intermediate state and keep track of the accession ids retrieved between runs of `accession` and `dataset`.

### dataset

```bash
./bpctl dataset [flags]
```

The dataset command will retrieve a list of accessionIDs and send a request to the sda api `POST /dataset`

Will try to read from `<DATASET_FOLDER>-fileIDs.txt` to identify the files to be included in a dataset. If the file cannot be found it will make a call to `GET /user/files?path_prefix=<DATASET_FOLDER>` to find them and send a request to `POST /dataset/create` with the files.

### mail

```bash
./bpctl mail [flags]
```

Will send email notifications about dataset finalization to a fixed list of parties with information and attachments specifically for each.

`bigpicture submission`: recieves a mail about dataset creation and attachments with `dataset.txt` and `policy.txt`

`bigpicture PO`: recieves a mail about dataset creation and attachments with `rems.txt`, `dataset.txt` and `policy.txt`

`uploader`: recieves a mail confirming the creation of the dataset is completed with attachments `<datasetFolder>-stableIDs.txt`

Attachements such as `rems.txt`, `dataset.txt` and `policy.txt` needs to be available under `--data-directory` when running as a job. During the job process it will produce  a `<datasetFolder>-stableIDs.txt` and include it. If running the mail as a standalone command the `<datasetFolder>-stableIDs.txt` also needs to be available under `--data-directory`.

### render

```bash
./bpctl render [flags]
```

Will render a 'opinionated' kubernetes yaml manifest that defines a `job` resrouce. Fields specific for a given dataset is populated from `config.yaml` while other big picture specific deployment fields such as `CLIENT_API_HOST`, `CERT_SECRET_NAME` and similar are hard coded. This is not a generic template that is meant to fit multiple purposes, It's specifically made to fit the big picture kubernetes deployment in NBIS.

### status

```bash
./bpctl status [flags]
```

Will get a list of files for the `USER_ID` that resides in `DATASET_FOLDER` and print a report of how many files are in each status, e.g.

```
- verified: 2012
- uploaded: 124
- submitted: 2
```

Use `--status`/`-s` to instead list the file IDs currently in a specific status, one per line:

```bash
./bpctl status --status verified
```

Add `--sql` to format that list as a SQL `IN` clause instead, ready to paste into a `psql` query:

```bash
./bpctl status --status verified --sql
# ('id1', 'id2', 'id3')
```

### landingpage

```bash
./bpctl landingpage [flags]
```

Will look for any objects in `S3_INBOX_BUCKET` that have a `LANDING_PAGE` in their part and try to get them, decrypt them using crypt4gh and finally put the decrypted objects to a specified location in `METADATA_INBOX_BUCKET`. If the put is successfull it will remove the encrypted object from the `S3_INBOX_BUCKET`

### job

```bash
./bpctl job <expectedFiles> [flags]
```

Will run all the above steps in order of ingest -> accession -> dataset and ensure the expectedFiles are keept between each stage where expectedFiles represents a integer value of the ammount of files that is expected to be present in the finalized dataset. If a dataset completes without error the job command will finish with sending out email notifications and move eventual landing pages from the inbox bucket to the public metadata bucket.

### testing

Unit tests using [pkg.go.dev/testing](https://pkg.go.dev/testing) 

Running all tests:
```bash
go test ./...
```
