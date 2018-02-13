
CONFIGMAPS = photond1 photond2 photond3 photond4

configmap:
	$(patsubst %,kubectl delete configmap % | true;,$(CONFIGMAPS))
	$(foreach config,$(CONFIGMAPS),$(subst %,$(config),kubectl create configmap % --from-file=./%;))
