# batch-submit

Submit several BigPicture datasets from the inbox in one go. For each dataset
the scripts validate it, submit it for ingestion, and (optionally) reply to the
uploader's ticket with a "validation passed" message and add the tag
`validation successful`.

| Script | Purpose |
|---|---|
| `batch_submit.sh` | Reads a list file and runs `submit_dataset.sh` for each dataset |
| `submit_dataset.sh` | Validates and submits one dataset, then replies to the ticket |
| `setup_credentials.sh` | Creates the missing s3cmd config from vault |
| `get_inbox_filelist.sh` | Writes the S3 listing of the inbox (cached for a day) |
| `get_filelist_for_datasetfolder.sh` | Writes the file list of one dataset folder |
| `send_email_through_bptickets.py` | Sends the reply through the ticketing system, see [README_send_email.md](README_send_email.md) |
| `example_email.yaml` | Example config for `send_email_through_bptickets.py` |

## Before you start

Required, and checked by `batch_submit.sh` before the first dataset (nothing is submitted
if one is missing):

| Requirement | How to get it |
|---|---|
| `s3cmd`, `kubectl`, `vault`, `python3` in `PATH` | install them |
| Vault login | `vault login ...` (`VAULT_ADDR` defaults to `https://vault.nbis.se`). Checked with `vault token renew`. |
| `KUBECONFIG` | export it, or have `~/.sda/kubeconfig_bp.yaml` (the default). The file must exist; create it with `cluster_v2/k8s_cluster/scripts/get_kubeconfig.sh` in BigPicture-Deployment. |
| `ACCESS_TOKEN` (admin API token for the validator) | export it, or have the `access_token` line in `~/.sda/s3cmd-bp-download.conf`, which is read automatically |
| `TICKET_USER_ID` | only to reply to tickets: export your agent id, the number in `https://tickets.nbis.se/users/profile/<id>` |

The validator is `../validator/validator.sh` in this repository, found relative to the
scripts. No init file is needed.

### Credentials created from vault

You do not have to create these by hand:

| What | Created from | When |
|---|---|---|
| `~/.sda/s3cmd-bp-master-inbox.conf` (mode 600) | `bp-secrets/S3_keys/STO2/inbox` (keys) and `bp-secrets/S3_keys/STO2` (endpoint) | if the file is missing |
| `TICKET_API_KEY` | `bp-secrets/bp-tickets`, field `apikey` | only when a list row has a conversation id, and the key is not exported. It is kept in memory and not written to disk. |

An existing file is never overwritten. To rebuild the s3cmd config, for example after the
keys were rotated, run `./setup_credentials.sh --force`.

### Optional settings

| Variable | Default |
|---|---|
| `TICKET_MAILBOX_ID` | `2` |
| `TICKET_URL` | `https://tickets.nbis.se` |
| `TICKET_API_KEY` | read from vault; export it to use another key |

Example, once per shell session:

```bash
export TICKET_USER_ID=9
```

## Quick start

1. Create a list file, one dataset per line, fields separated by `|`:

   ```
   DATASET_AAAAAAA|aaaaaaaaa.aaaaaa@dummy.com|Dummy User|889
   DATASET_BBBBBBB|bbbbbbbbb.bbbbbb@dummy.com|Other User
   # lines starting with # and empty lines are ignored
   ```

   | Field | Meaning |
   |---|---|
   | `DATASET_FOLDER` | dataset folder name in the inbox |
   | `MAIL_UPLOADER` | uploader's email |
   | `MAIL_UPLOADER_NAME` | uploader's name, as in the ticket |
   | `CONVERSATION_ID` | optional; the number in `https://tickets.nbis.se/conversation/<ID>`. Without it, no reply is sent. |

2. Go to an empty working directory and run:

   ```bash
   cd ~/submissions/2026-11-09
   /path/to/sda-bpctl/batch-submit/batch_submit.sh list.txt
   ```

Each dataset gets its own subfolder (`./DATASET_AAAAAAA/`) with the full output in
`submit_dataset.log`. The inbox listing `bpprod-inbox-all.s3.txt` is stored in the
working directory and reused for 24 hours.

## What happens for each dataset

1. The inbox listing is refreshed if it is older than a day, and the uploader's
   folder is found in it.
2. The dataset's file list is written.
3. **Dry-run validation.** The dataset is submitted only if the validator reports
   `All checks passed`. Otherwise it stops here and nothing is changed.
4. The `xml-files` and `xsd-files` folders are moved to `bak/` and the dataset is submitted
   to production.
5. If a conversation id was given: a reply is sent to the uploader and the ticket is
   tagged `validation successful`. The conversation is assigned to `TICKET_USER_ID`.
6. `<DATASET_FOLDER>.info.conf` is written (dataset id, uploader, organisation).

The reply is sent only after the dataset is submitted. The settings for the reply are
checked **before** anything is submitted, so a missing API key stops the run early.

The summary at the end of the batch lists the datasets that succeeded, failed, were
replied to, or were submitted but whose reply failed.

## Run one dataset

```bash
mkdir DATASET_AAAAAAA && cd DATASET_AAAAAAA        # the folder name must match -d
/path/to/sda-bpctl/batch-submit/submit_dataset.sh \
    -d DATASET_AAAAAAA -n "Dummy User" -m aaaaaaaaa.aaaaaa@dummy.com -c 889
```

`-c` is optional. The directory name must equal the dataset folder name, otherwise the
script refuses to run.

## Exit codes and what to do

| Code | Meaning | Action |
|---|---|---|
| 0 | Dataset submitted (and replied, if asked) | none |
| 1 | Failed before the submission (check, validation, missing tool) | Read the log, fix, run again |
| 3 | **Dataset is submitted, but the reply failed** | Do **not** submit again. Reply by hand in the ticket and add the tag. |

`batch_submit.sh` reports these per dataset and exits 1 if any dataset failed or any
reply failed. Datasets that already succeeded are not repeated automatically: remove
them from the list file before you run it again.

## Common problems

- **`Current directory ... does not match requested DATASET_FOLDER`**: run
  `submit_dataset.sh` from a folder named like the dataset. `batch_submit.sh` does this for you.
- **`You must log in to https://vault.nbis.se`**: run your vault login again.
- **`KUBECONFIG file ... not found`** or **`ACCESS_TOKEN is not set`**: see the requirements above.
- **`TICKET_USER_ID must be set to a number`**: export your agent id, see above. Only needed
  when the list has conversation ids.
- **`could not read TICKET_API_KEY from vault`**: your vault policy may not allow
  `bp-secrets/bp-tickets`. Export `TICKET_API_KEY` yourself instead.
- **`Could not determine USER_ID`**: the folder is not in the inbox listing. Delete
  `bpprod-inbox-all.s3.txt` to force a new listing and check the folder name.
- **Validation failed**: read `DATASET_X/submit_dataset.log`. Nothing was submitted.
- **Reply failed, exit code 3**: see the table above. The error message from the
  ticketing system is in the log.

## Send other emails

`send_email_through_bptickets.py` also works on its own: new emails, replies, CC,
tags, HTML. See [README_send_email.md](README_send_email.md) and `example_email.yaml`.
