# Nighly tests

This folder contains different nighly tests run by Cloud Build

## Repositories and artifacts

1. celo-blockchain: Contains the cloudbuild job definitions and the helm value files.
1. celo-infrastructure: Contains the helm charts and helm artifacts using in the tests.
1. kliento: Contains the health-check script used in the multiversion test.

## Trigger executor

The cloudbuilds cron jobs are triggered using a Kubernetes cronjob in `celo-networks-dev` cluster and `nightly-tools` namespace.
The cronjob uses [Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) to authenticate as a Google Cloud IAM Service Account.
If you want to add a new cron job for a new Cloud Build Job, the easiest path is duplicating existing cronjob and change only the name, the schedule and the job triggered (the same service account can be reused)

Here are the steps followed to setup the trigger executor.

1. Create the new Google IAM Service account.

```bash
gcloud iam service-accounts create nightly-cloudbuild-trigger --project celo-testnet --display-name "Service Account for triggering nightly cloudbuild jobs"
```

1. Assign role "Cloudbuild editor" to allow triggering the jobs

```bash
gcloud projects --project celo-testnet add-iam-policy-binding celo-testnet \
--member serviceAccount:nightly-cloudbuild-trigger@celo-testnet.iam.gserviceaccount.com \
--role roles/cloudbuild.builds.editor
```

1. Create a service account in k8s to use with the Workload identity

```bash
kubectl create namespace nightly-tools
kubectl create serviceaccount --namespace nightly-tools nightly-cloudbuild-trigger
```

1. Annotate the K8s service account with the GCE service account, and add the policy binding to allow impersonate

```bash
kubectl annotate serviceaccount \
  --namespace nightly-tools nightly-cloudbuild-trigger \
  iam.gke.io/gcp-service-account=nightly-cloudbuild-trigger@celo-testnet.iam.gserviceaccount.com

gcloud iam service-accounts add-iam-policy-binding --project celo-testnet\
  --role roles/iam.workloadIdentityUser \
  --member "serviceAccount:celo-testnet.svc.id.goog[nightly-tools/nightly-cloudbuild-trigger]" nightly-cloudbuild-trigger@celo-testnet.iam.gserviceaccount.com
```

1. Create the new k8s cronjob for triggering the jobs

```bash
cat <<EOF | kubectl apply -f -
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  creationTimestamp: null
  name: trigger-load-test
  namespace: nightly-tools
spec:
  jobTemplate:
    metadata:
      creationTimestamp: null
      name: trigger-load-test
      namespace: nightly-tools
    spec:
      template:
        metadata:
          creationTimestamp: null
        spec:
          serviceAccountName: nightly-cloudbuild-trigger
          containers:
          - image: google/cloud-sdk:latest
            name: trigger-load-test
            command:
            - "/bin/sh"
            - "-c"
            args:
            - |
               gcloud alpha builds triggers run --branch=jcortejoso/nightly test-geth-multiversion
            resources: {}
          restartPolicy: OnFailure
  schedule: 0 8 * * *
status: {}
EOF
```

## Backup chain

When deploying the testnet, the helm variable `geth.use_gstorage_data` is used to determine if the testnet should copy an existing chain from a Google Storage Bucket. By default the bucket used is `testnet_sample_chain` in `celo-testnet` project, but this can be configured using helm variables.

### Current setup

The testnet helm chat will add an init container to the validators and proxies pods that will download and copy the chain data from the Google Storage Bucket. In ordet to authenticate and authorize this pods in Google Cloud, they need to be assigned a Kubernetes service account that is mapped with a Google IAM service account who has the permissions to download from the bucket (check [Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) for more internal details of this).

Currently the Google IAM Serive account `nighly-storage-access` is already configured with the permissions to access to the chain bucket `testnet_sample_chain`. To allow this account be assigned to the Kubernetes service account `gcloud-storage-access` in namespace `nightlyloadtests`, you need to run the command:

```bash
gcloud iam service-accounts add-iam-policy-binding --project celo-testnet\
  --role roles/iam.workloadIdentityUser \
  --member "serviceAccount:celo-testnet.svc.id.goog[nightlyloadtests/gcloud-storage-access]" nightly-storage-access@celo-testnet.iam.gserviceaccount.com
```

Notice that the kubernetes service account must have the annotation `iam.gke.io/gcp-service-account=nightly-storage-access@celo-testnet.iam.gserviceaccount.com` to allow the impersonation with Workload Identity. You can setup this label in the helm/kubernetes template or labeling it manually:

```bash
kubectl annotate serviceaccount \
  --namespace $namespace gcloud-storage-access \
  iam.gke.io/gcp-service-account=nightly-storage-access@celo-testnet.iam.gserviceaccount.com
```

The Kubernetes Service Account and the Kubernetes Namespace do not need to exist prior to run these commands, so you can setup it in advance and then create the namespace and service account using helm during the deployment (also the k8s namespace and service account can be deleted and the policy binding remains).

### Creating a new backup

If you want to make a new backup so it can be used as source when restorning the chain, the high level steps would be:

1. Deploy a new testnet with the desired setup (genesis file, epoch block, block time, network id, mnemonic, number of validators...)

1. Run the migrations in the new testnet, configured as desired.

1. Stop the testnet, and copy the folder `/root/.celo/celo/chain` from one validator to the bucket

## Helm charts

Please refer to https://github.com/celo-org/infrastructure/ to check details about the helm charts used in this tests.
