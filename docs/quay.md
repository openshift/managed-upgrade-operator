#### Prepare quay repo

- Create new repo in quay.io:
  - Navigate to https://quay.io/new/ and create new empty repository named `managed-upgrade-operator`

- Create service account for the repo:
  - Navigate https://quay.io/repository/<quay_username>/managed-upgrade-operator?tab=settings
  - Click dropdown icon under "select user" and create new robot account and give `write` permission to the repo

- Generate account token:
  - Click on robot account name and select `docker login`

- Login with a service account:

```shell
podman login -u="<QUAY_SERVICE_ACCOUNT>" -p="<QUAY_SERVICE_ACCOUNT_TOKEN>" quay.io 
```
