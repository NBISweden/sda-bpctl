# batch-submit

Submit several BigPicture datasets from the inbox in one go. For each dataset the
scripts validate it, submit it for ingestion and, if you give a ticket number, reply
to the uploader with a "validation passed" message and add the tag
`validation successful` to the ticket.

## Get started

### 1. Check that you have these

- Tools in your `PATH`: `s3cmd`, `vault`, `kubectl`, `bpctl`, `xmllint`, `crypt4gh`,
  `jq`, `python3`.
- A vault login: `vault login ...`
- A working kubeconfig. By default `~/.sda/kubeconfig_bp.yaml` is used; set `KUBECONFIG`
  to use another. `kubectl get ns` must work with it. If you have none, create it with
  `cluster_v2/k8s_cluster/scripts/get_kubeconfig.sh` in BigPicture-Deployment.
- `ACCESS_TOKEN`: `export ACCESS_TOKEN=...`, or have the `access_token` line in
  `~/.sda/s3cmd-bp-download.conf` (read automatically).
- Only if you want to reply to tickets: your ticketing agent id, the number in
  `https://tickets.nbis.se/users/profile/<id>`:

  ```bash
  export TICKET_USER_ID=9
  ```

The scripts check all of this before the first dataset and stop with a clear message if
something is missing, so nothing is submitted by mistake. The s3cmd config
(`~/.sda/s3cmd-bp-master-inbox.conf`) and the ticket API key are fetched from vault
for you, see [Credentials](#credentials).

### 2. Write a list file

One dataset per line, fields separated by `|`:

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
| `CONVERSATION_ID` | optional: the number in `https://tickets.nbis.se/conversation/<ID>`. Without it, no reply is sent. |

### 3. Run it

Use an empty working directory:

```bash
mkdir -p ~/submissions/2026-11-09 && cd ~/submissions/2026-11-09
/path/to/sda-bpctl/batch-submit/batch_submit.sh list.txt
```

Each dataset gets a subfolder (`./DATASET_AAAAAAA/`) with the full output in
`submit_dataset.log`. At the end, a summary lists what succeeded, failed, was replied to,
or was submitted but not replied to.

### 4. Check the result

| Exit code of a dataset | Meaning | What to do |
|---|---|---|
| 0 | Submitted (and replied, if asked) | nothing |
| 1 | Failed **before** submitting (a check, the validation) | Read `DATASET_X/submit_dataset.log`, fix, run again |
| 3 | **Submitted, but the reply failed** | Do **not** submit again. Reply by hand in the ticket and add the tag. |

`batch_submit.sh` exits 1 if any dataset failed or any reply failed. Finished datasets
are not skipped on a second run: remove them from the list file first.

## What happens for each dataset

1. The inbox listing is refreshed if it is older than a day (`bpprod-inbox-all.s3.txt` in
   the working directory), and the uploader's folder is found in it.
2. The dataset's file list is written.
3. **Dry-run validation.** The dataset is submitted only if the validator reports
   `All checks passed`. Otherwise it stops here and nothing is submitted.
4. `xml-files` and `xsd-files` are moved to `bak/` and the dataset is submitted to production.
5. With a conversation id: a reply is sent to the uploader, the ticket gets the tag
   `validation successful`, and it is assigned to `TICKET_USER_ID`.
6. `<DATASET_FOLDER>.info.conf` is written (dataset id, uploader, organisation).

The reply is sent only after the submission, and the settings for it are checked
**before** anything is submitted.

## Run one dataset

```bash
mkdir DATASET_AAAAAAA && cd DATASET_AAAAAAA        # the folder name must match -d
/path/to/sda-bpctl/batch-submit/submit_dataset.sh \
    -d DATASET_AAAAAAA -n "Dummy User" -m aaaaaaaaa.aaaaaa@dummy.com -c 889
```

`-c` is optional. The script refuses to run if the directory name differs from the
dataset folder. `submit_dataset.sh -p` only runs the prerequisite checks (add `-t` to
check the ticket settings too).

## Credentials

Created for you when needed:

| What | Source in vault | When |
|---|---|---|
| `~/.sda/s3cmd-bp-master-inbox.conf` (mode 600) | `bp-secrets/S3_keys/STO2/inbox` (keys), `bp-secrets/S3_keys/STO2` (endpoint) | the file is missing |
| `TICKET_API_KEY` | `bp-secrets/bp-tickets`, field `apikey` | a list row has a conversation id and the key is not exported. Kept in memory only. |

An existing file is never overwritten. After a key rotation, rebuild it with
`./setup_credentials.sh --force`.

Optional settings: `TICKET_MAILBOX_ID` (default `2`), `TICKET_URL` (default
`https://tickets.nbis.se`), `VAULT_ADDR` (default `https://vault.nbis.se`). An exported
`TICKET_API_KEY` is used instead of the one in vault.

## Troubleshooting

- **`You must log in to https://vault.nbis.se`**: run `vault login` again.
- **`kubectl get ns` failed**: the kubeconfig is expired or the cluster is unreachable
  (VPN?). Create a fresh kubeconfig.
- **`KUBECONFIG file ... not found`** or **`ACCESS_TOKEN is not set`**: see step 1.
- **`TICKET_USER_ID must be set to a number`**: export your agent id (step 1). Only needed
  when the list has conversation ids.
- **`could not read TICKET_API_KEY from vault`**: your vault policy may not allow
  `bp-secrets/bp-tickets`. Export `TICKET_API_KEY` yourself.
- **`Could not determine USER_ID`**: the folder is not in the inbox listing. Delete
  `bpprod-inbox-all.s3.txt` to get a new listing and check the folder name.
- **`Current directory ... does not match requested DATASET_FOLDER`**: only when running
  `submit_dataset.sh` by hand. `batch_submit.sh` does it for you.

## Files in this folder

| File | Purpose |
|---|---|
| `batch_submit.sh` | Reads the list file and runs `submit_dataset.sh` for each dataset |
| `submit_dataset.sh` | Checks, validates and submits one dataset, then replies to the ticket |
| `setup_credentials.sh` | Creates the missing s3cmd config from vault |
| `get_inbox_filelist.sh` | Writes the S3 listing of the inbox (cached for a day) |
| `get_filelist_for_datasetfolder.sh` | Writes the file list of one dataset folder |
| `send_email_through_bptickets.py` | Sends the reply; also usable alone, see [README_send_email.md](README_send_email.md) |
| `example_email.yaml` | Example config for `send_email_through_bptickets.py` |
