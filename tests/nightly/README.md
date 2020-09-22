# Nighly tests

This folder contains different nighly tests run by Cloud Build


## Copying chain files from gstorage

If desired a new testnet can be started with an exiting chain data that is downloaded when it starts from google cloud storage.

### Allowing a new namespace/test to download the chain from google storage

The authentication of the pods in Google Cloud is done using [Workload Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity).
The Google Service Account `nighly-storage-access` is already configured with the permissions to access to the chain bucket `testnet_sample_chain`.
In order to authorize a new namespace to use this Google Service Account, it is needed to create a Kubernetes Service Accounti (`gcloud-storage-access`) in that namespace,
and authorize the binding between the Google Service Account and the Kubernetes service account, running the next commands:

```bash
export namespace=<namepace>

kubectl create serviceaccount -n $namespace gcloud-storage-access

gcloud iam service-accounts add-iam-policy-binding --project celo-testnet\
  --role roles/iam.workloadIdentityUser \
  --member "serviceAccount:celo-testnet.svc.id.goog[$namespace/gcloud-storage-access]" nightly-storage-access@celo-testnet.iam.gserviceaccount.com

kubectl annotate serviceaccount \
  --namespace $namespace gcloud-storage-access \
  iam.gke.io/gcp-service-account=nightly-storage-access@celo-testnet.iam.gserviceaccount.com
```

### Backing up chain data in google cloud storage

Current configured bucket for chain data is `testnet_sample_chain`. To replace the chain data there, it is needed to deploy a new testnet and copying the folder `/root/.celo/celo`
from one validator to the bucket `testnet_sample_chain`. If the genesis or some network parameters are updated, these have to be updated too.


