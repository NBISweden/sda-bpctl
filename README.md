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

The `job` command is meant to try and run all the steps of the dataset submission process in order; ingest -> accession -> dataset, waiting for each step to complete before starting the next. It will need environment variables to configure and will need to be adjusted for the environment to run in. See the [job](#job) section for details.

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

The config file format is detected from its file extension, so use `.yaml` or `.yml` for a YAML file. A file with an unsupported extension such as `.conf` is not read at all: bpctl logs `failed to read config file, falling back to environment variables` and then fails validation, e.g. with `DATASET_FOLDER requiered`.

Since environment variables take priority, single values can be overridden for one run without editing the config file:

```bash
CLIENT_API_HOST=https://staging-api.bp.nbis.se ./bpctl status --config config.yaml
```

see the `config.yaml.example` for a base template with what fields to fill. The example shows a minimal config. The table below shows all possible values that can be configured:
| Name                              | Default                           |
| --------------------------------- | --------------------------------- |
| DATASET_FOLDER                    | none                              |
| DATASET_ID                        | none                              | 
| USER_ID                           | none                              |
| SSL_CA_CERT                       | none                              |
| JOB_TIMEOUT                       | 4320 (minutes, i.e. 72 hours)     |
| JOB_POLL_RATE                     | 180 (minutes, i.e. 3 hours)       |
| JOB_DATA_DIRECTORY                | "/data"                           |
| CLIENT_API_HOST                   | "https://api.bp.nbis.se"          |
| CLIENT_ACCESS_TOKEN               | none                              |
| CERT_SECRET_NAME                  | "sda-sda-svc-api-certs"           |
| STORAGE_SECRET_NAME               | "sda-bpctl-storage"               |
| MAIL_SECRET_NAME                  | "sda-bpctl-mail"                  |
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

Attachements such as `rems.txt`, `dataset.txt` and `policy.txt` needs to be available under `--data-directory` when running as a job. During the job process it will produce  a `<datasetFolder>-stableIDs.txt` and include it. If running the mail as a standalone command the `<datasetFolder>-stableIDs.txt` also needs to be available under `--data-directory`. With `--dry-run`, the mails are rendered and their attachments checked, but nothing is sent. See [sending the notification mails manually](#sending-the-notification-mails-manually).

### render

```bash
./bpctl render [flags]
```

Will render a 'opinionated' kubernetes yaml manifest that defines a `job` resrouce. Fields specific for a given dataset is populated from `config.yaml` while other big picture specific deployment fields such as `CLIENT_API_HOST`, `CERT_SECRET_NAME` and similar are hard coded. This is not a generic template that is meant to fit multiple purposes, It's specifically made to fit the big picture kubernetes deployment in NBIS.

The rendered manifest has no namespace, so set it when applying, e.g. `kubectl -n sda-prod apply -f job.yaml`. The API host (`https://sda-sda-svc-api:8080`) and the `release: sda` label are hard coded to the `sda-prod` deployment and are not read from the config. In `sda-staging` the API service is `pipeline-sda-svc-api` and the label is `release=pipeline`, so a manifest rendered for staging has to be edited before it is applied.

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
./bpctl job [flags]
```

Will run all the above steps in order. Ingestion, accession and mapping files to a dataset are processed asynchronously by the SDA, so the job polls the API every `JOB_POLL_RATE` minutes and waits for each step to complete before starting the next:

1. ingest: wait until all dataset files are `verified`
2. accession: wait until all dataset files are `ready`
3. dataset: wait until all dataset files are mapped to `DATASET_ID`
4. landing page: move eventual landing pages from the inbox bucket to the public metadata bucket
5. mail: check that `<DATASET_FOLDER>-stableIDs.txt` lists every dataset file, then send email notifications

"Dataset files" are all files under `DATASET_FOLDER`, except files under `PRIVATE` and `LANDING_PAGE`. The job fails if any of them ends up in `error` status, if a step does not complete within `JOB_TIMEOUT` minutes, or if the stable IDs list is incomplete (see below). Failing landing page and mail steps are only logged as warnings.

The job can be re-run after it failed or timed out; it picks up from where the previous run stopped.

#### incomplete stable IDs list after a re-run

The mail to the uploader has `<DATASET_FOLDER>-stableIDs.txt` attached, which lists the stable ID of every file in the dataset. The job writes this file inside the job pod from the files the API returns, and the API no longer returns files that are part of a dataset. So when a previous run had already mapped files to the dataset, the re-run can not write a complete list:

- if some files were mapped by the previous run, the file only lists the files mapped during the re-run
- if all files were mapped by the previous run, the file is not written at all

Before sending any mail, the job checks that the file lists every dataset file. If it does not, no mail is sent and the job fails with e.g.:

```text
Error: not sending mail notifications: stable IDs file /data/DATASET_ABC-stableIDs.txt lists 120 of 500 dataset files
```

or `... stable IDs file /data/DATASET_ABC-stableIDs.txt does not exist, expected 500 dataset files`. The dataset itself is complete at this point, and the landing page step has already run; only the notification mails are missing. Send them manually as described below.

#### sending the notification mails manually

`bpctl mail` sends the same three mails as the job. Run it from the dataset's working directory created by `validator.sh` (see [validator/README.md](validator/README.md)), on a machine that can reach the SMTP server (`MAIL_SMTP_HOST`, default `mail.nbis.se:587`). That directory already has everything else `bpctl mail` needs:

- `config.yaml` with `USER_ID`, `DATASET_ID`, `DATASET_FOLDER` and the `MAIL_UPLOADER*` values
- `data/xml/` with `dataset.txt`, `rems.txt` and `policy.txt`

`bpctl mail` reads `config.yaml` and `data/` by default, so only the stable IDs list and the mail account credentials are missing.

1. Rebuild the complete stable IDs list from the database and save it to `data/`. The output has the same format as the file the job writes: one `<stable ID> <inbox path>` per line.

    ```bash
    cd <working directory of DATASET_FOLDER>

    PRIMARY=$(kubectl -n sda-prod get pods -l cnpg.io/instanceRole=primary -o name)
    kubectl -n sda-prod exec $PRIMARY -- psql -U postgres -d sda -At -F ' ' -c "
      SELECT f.stable_id, f.submission_file_path
      FROM   sda.files f
      JOIN   sda.file_dataset fd ON fd.file_id = f.id
      JOIN   sda.datasets d ON d.id = fd.dataset_id
      WHERE  d.stable_id = '<DATASET_ID>'
      ORDER  BY f.submission_file_path" > data/<DATASET_FOLDER>-stableIDs.txt

    wc -l < data/<DATASET_FOLDER>-stableIDs.txt   # should equal the number of files in the dataset
    ```

2. Get the mail account credentials from the cluster:

    ```bash
    export MAIL_ADDRESS=$(kubectl -n sda-prod get secret sda-bpctl-mail -o jsonpath='{.data.MAIL_ADDRESS}' | base64 -d)
    export MAIL_PASSWORD=$(kubectl -n sda-prod get secret sda-bpctl-mail -o jsonpath='{.data.MAIL_PASSWORD}' | base64 -d)
    ```

3. Do a dry run. It renders all three mails and checks that every attachment exists and is not empty, but sends nothing:

    ```bash
    bpctl mail --dry-run
    ```

4. Send the mails:

    ```bash
    bpctl mail
    ```

5. Check the bp-notify mailbox. Every notification mail is sent with a BCC to the sender address (`MAIL_ADDRESS`), so it holds a copy of each mail. The copy of the mail to the uploader, "Successful Ingestion of Your Dataset Submission", should have the complete `<DATASET_FOLDER>-stableIDs.txt` attached.

The job requires an SDA API version that provides `GET /dataset/{datasetID}`, i.e. `v4.0.0` or later.

The job logs to stderr. To follow the log and keep it in a file at the same time:

```bash
./bpctl job --config config.yaml 2>&1 | tee job.log
```

### testing the job in staging

The job can be run locally against the staging API. Build the binary from the current source first, so that you test the latest code:

```bash
go build -o bpctl .
```

Create a config file for staging, e.g. `config-staging.yaml`:

```yaml
USER_ID:
DATASET_ID:
DATASET_FOLDER:
CLIENT_ACCESS_TOKEN:
CLIENT_API_HOST: "https://staging-api.bp.nbis.se"
MAIL_SMTP_HOST: "no-mail.invalid"
JOB_DATA_DIRECTORY: "./data"
JOB_POLL_RATE: 2      # check every 2 minutes
JOB_TIMEOUT: 1200     # give up after 20 hours
```

- `CLIENT_ACCESS_TOKEN` must be a token for the staging environment.
- `CLIENT_API_HOST` points the job to the staging API instead of the default production API.
- `MAIL_SMTP_HOST` is set to a host that never resolves, since the mail recipients are hard coded and include real people. The mail step then fails with a warning and no mail is sent.
- `JOB_DATA_DIRECTORY` must be a writable local directory. The default `/data` only exists in the kubernetes job, and the job fails after the dataset step if the stable IDs file cannot be written.
- `JOB_POLL_RATE` and `JOB_TIMEOUT` are shortened from the production defaults (3 and 72 hours).

Then run the job:

```bash
./bpctl job --config config-staging.yaml 2>&1 | tee job.log
```

Expected results:

- the dataset files reach `verified` and `ready`, which can be followed with `./bpctl status --config config-staging.yaml`
- all files are mapped to `DATASET_ID`
- `<JOB_DATA_DIRECTORY>/<DATASET_FOLDER>-stableIDs.txt` is written
- the landing page step logs `could not complete landingpage`, since no crypt4gh key (`C4GH_SEC_PEM`) or S3 credentials are configured
- the mail step logs `could not complete mail notifications`, and no mail is sent
- the job ends with `dataset submission completed!`

### testing

Unit tests using [pkg.go.dev/testing](https://pkg.go.dev/testing) 

Running all tests:
```bash
go test ./...
```
