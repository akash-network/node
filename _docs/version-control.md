# Merging into `mainnet/main`

As new mainnet need to be released, quite often `mainnet/main` branch is far behind the `master`.
This is so far cleanest solution we have found to perform merge without conflicts as well as keeping history

```shell
git checkout master
git merge -s ours mainnet/main
git checkout mainnet/main
git merge master
git push origin mainnet/main
```
