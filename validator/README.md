# Script description

The `validator.sh` script can be used for

- Validating the folder structure of a dataset
- Validating all the metadata files against the xsd schemas
- Checking that all the files referenced in the `images.xml` file exist in the inbox
- Checking for exta or missing files in the inbox
- Adding the datset id in the `dataset.xml` file and replacing the original one
- Moving `PRIVATE` and `LANDING_PAGE` folders in the metadata bucket
- Creating Kubernetes job manifest and deploy the job in case of prod cluster

## Prerequisites & Working Directory Recommendation

Before running the script, ensure you are logged into Vault and that `bpctl` is available in your `$PATH`.

### Recommended Directory Setup:

It is strongly recommended to isolate each dataset run by creating a separate working directory named after the `DATASET_FOLDER` (e.g., `DATASET_aaaaaaa`) and running the script from within that directory or passing WORKDIR:

```bash
mkdir DATASET_aaaaaaa
cd DATASET_aaaaaaa
/path/to/validator.sh -c prod -u <username> -d DATASET_aaaaaaa --dry-run
```

## Standard Workflow

The recommended standard workflow consists of two main steps:

1. Dry Run (Pass 1): Run the script with the `--dry-run` flag first to verify dataset structure and metadata without altering files or moving data.
2. Ingestion (Pass 2): Run the script again **without** the `--dry-run` flag to execute the full modification, metadata transfer, and job deployment.

## Running the script

### Step 1: Validation / Dry Run Check

Run a dry run to check whether the validation for the dataset passes. The data will be downloaded to your local working directory for evaluation, but no files will be permanently modified or moved in S3.

```bash
/path/to/validator.sh -c <prod-or-staging> -u <username> -d <dataset-folder> --dry-run
```

Parameters:

- `-c`: Cluster name (prod or staging)
- `-u`: Username folder in the inbox
- `-d`: Dataset folder name (must follow the pattern DATASET_{identifier})

>**Note:** If you only need to run schema validation without checking deployment parameters, you can also use `--validation-only`.

After completing a dry run or clean-up, you should clear temporary files generated in your WORKDIR by running:

```bash
/path/to/validator.sh --clean
```

### Step 2: Full Ingestion

Once the dry run passes successfully, run the script **without** `--dry-run` to proceed with dataset modification, metadata movement, and automated ingestion setup.
Ensure your `KUBECONFIG` is exported (required for production deployment):

```bash
export KUBECONFIG=/path/to/kubeconfig.yaml
```

Trigger the ingestion process:

```bash
/path/to/validator.sh -c prod -u <username> -d <dataset-folder> -n "<uploader-name>" -e <uploader-email> -t <admin-token>
```

Parameters:

- `-c`: Must be `prod` (automated ingestion works **ONLY** for production)
- `-u`: Username folder name in the inbox
- `-d`: Dataset folder name (`DATASET_{identifier}`)
- `-n`: Full name of the uploader (as listed in the ticket)
- `-e`: Email address of the uploader
- `-t`: Download token for accessing the Admin API

Upon completion, a Kubernetes manifest (`<dataset-folder>.yaml`) will be generated in your working directory and applied to the `sda-prod` cluster.
