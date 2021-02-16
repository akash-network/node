# Authentication with mTLS

For tenant it is important to send manifest to the right provider as well as for provider
to ensure only owners can access their deployments.
Thus each account must [create](#manage-certificates) certificate prior deploying workload or starting the provider.

**Note**
In this guide `--from` is referring to the key `main` which has been previously created with `akash key add`. Consider changing to the name of yours.
```yaml
- name: main
  type: local
  address: akash1gp3scyd8aye3z8szf3mpqzgsg4csyplcqehxus
  pubkey: akashpub1addwnpepq0np6xltudgnau39046qtty3k46gzd482884hqcfxvzpyf2ttnr8ue3hc55
  mnemonic: ""
  threshold: 0
  pubkeys: []
```
## Manage certificates
By default certificate is valid for 365 days from the moment of issuing

### Create
#### Client (aka tenant) certificate
```shell
akash tx cert create client --from=main
```

#### Provider certificate
It is important for provider to list same domain(s) as hostURI in provider attributes
For example if `HostURI: https://example.com` the `example.com` must be listed as one of the domains in the certificate 
```shell
#akash tx cert create server [list of domains provider is serving on] --from=main
akash tx cert create server example.com example1.com --from=main
```

Locally certificates and it's respective private key are stored in single file in akash home directory.
The name of the file is stated as `<address>.pem`. For example certificate created with key `main` the file will be named as
`akash1gp3scyd8aye3z8szf3mpqzgsg4csyplcqehxus.pem`

If file already exists user will be prompted to check if certificate already present on chain:
 - certificate is **not** on chain: user is prompted whether to commit or to leave as is
 - certificate is on chain: user prompted to revoke it or leave as is

To create certificate without being prompted use `--rie` flag (revoke if exists)

#### Custom expiration dates
Use following flags to set custom period of validity
 - `naf`: valid not after. value either number of days with `d` suffix `364d` or RFC3339 formatted timestamp
 - `nbf`: valid not before. value must be RFC3339 formatted timestamp
 
**Note** flags above are valid for both [client](#client-aka-tenant-certificate) and [server](#provider-certificate) certificates

##### example1
certificate valid for 180days after issuing
```shell
akash tx cert create client --from=main --naf=180d
```

##### example2
certificate valid for 180days after date of start
```shell
akash tx cert create client --from=main --naf="2022-03-19T18:35:03-04:00" --naf=180d
```

##### example3
certificate valid for 365days after date of start
```shell
akash tx cert create client --from=main --naf="2022-03-19T18:35:03-04:00"
```

### Revoke
```shell
akash tx cert revoke --from=main
```

```shell
akash tx cert revoke --from=main --serial=<serial #>
```

## Query
To query certificates for particular account
```shell
akash query cert list --owner="$(akash keys show main -a)"
```

To filter by state
```shell
akash query cert list --owner="$(akash keys show main -a)" --state=valid
akash query cert list --owner="$(akash keys show main -a)" --state=revoked
```
