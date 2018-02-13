
CONFIGMAPS 	 = photond1 photond2 photond3 photond4

configmap:
	$(patsubst %,kubectl delete configmap % | true;,$(CONFIGMAPS))
	kubectl create configmap photond1 --from-file=./photond1
	kubectl create configmap photond2 --from-file=./photond2
	kubectl create configmap photond3 --from-file=./photond3
	kubectl create configmap photond4 --from-file=./photond4
