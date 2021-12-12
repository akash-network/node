package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	v1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	"github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
	akashclient "github.com/ovrclk/akash/pkg/client/clientset/versioned"
	"github.com/ovrclk/akash/util/cli"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"

	v2beta1migrate "github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1/migrate"
)

const (
	flagCrdMigratePath = "k8s-crd-migrate-path"
	flagCrdRestoreOnly = "crd-restore-only"
)

func migrate() *cobra.Command {
	cmd := &cobra.Command{
		Use: "migrate",
	}

	cmd.AddCommand(migrateCRDs())

	return cmd
}

func migrateCRDs() *cobra.Command {
	cmd := &cobra.Command{
		Use: "crds",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doMigrateCRDs(cmd.Context(), cmd)
		},
	}

	cmd.Flags().String(FlagK8sManifestNS, "lease", "Cluster manifest namespace")
	if err := viper.BindPFlag(FlagK8sManifestNS, cmd.Flags().Lookup(FlagK8sManifestNS)); err != nil {
		return nil
	}

	cmd.Flags().String(flagCrdMigratePath, "./", "path to backup CRDs")
	if err := viper.BindPFlag(flagCrdMigratePath, cmd.Flags().Lookup(flagCrdMigratePath)); err != nil {
		return nil
	}

	cmd.Flags().Bool(flagCrdRestoreOnly, false, "proceed to restore step without making current backup")
	if err := viper.BindPFlag(flagCrdRestoreOnly, cmd.Flags().Lookup(flagCrdRestoreOnly)); err != nil {
		return nil
	}

	return cmd
}

func doMigrateCRDs(ctx context.Context, cmd *cobra.Command) error {
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	qc := mtypes.NewQueryClient(cctx)

	ns := viper.GetString(FlagK8sManifestNS)
	kubeConfig := viper.GetString(FlagKubeConfig)
	backupPath := path.Dir(viper.GetString(flagCrdMigratePath)) + "/crds"
	manifestsPath := backupPath + "/manifests"
	hostsPath := backupPath + "/hosts"
	restoreOnly := viper.GetBool(flagCrdRestoreOnly)

	config, err := openKubeConfig(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "kube: error building config flags")
	}

	ac, err := akashclient.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "kube: error creating manifest client")
	}

	var oldMani []v1.Manifest
	var oldHosts []v1.ProviderHost

	if !restoreOnly {
		isEmpty, err := isDirEmpty(manifestsPath)
		if os.IsNotExist(err) {
			isEmpty = true
		} else if err != nil {
			return err
		}

		if isEmpty {
			isEmpty, err = isDirEmpty(hostsPath)
			if os.IsNotExist(err) {
				isEmpty = true
			} else if err != nil {
				return err
			}
		}

		yes := true

		if !isEmpty {
			yes, err = cli.GetConfirmation(cmd, "backup already present. \"y\" to remove. \"N\" jump to restore. Ctrl+C exit")
			if err != nil {
				return err
			}

			if yes {
				_ = os.RemoveAll(backupPath)
			}
		}

		if yes {
			fmt.Println("checking manifests to backup")
			mList, err := ac.AkashV1().Manifests(ns).List(ctx, metav1.ListOptions{})
			if err != nil {
				return err
			}
			fmt.Println("checking providers hosts to backup")
			hList, err := ac.AkashV1().ProviderHosts(ns).List(ctx, metav1.ListOptions{})
			if err != nil {
				return err
			}

			if len(mList.Items) == 0 && len(hList.Items) == 0 {
				fmt.Println("no V1 objects found. nothing to do here")
				return nil
			}

			fmt.Printf("total to backup\n\tmanifests:      %d\n\tprovider hosts: %d\n", len(mList.Items), len(hList.Items))
			oldMani = mList.Items
			oldHosts = hList.Items

			fmt.Println("filtering closed leases")
			for i, mani := range oldMani {
				lease, err := qc.Lease(ctx, &mtypes.QueryLeaseRequest{ID: apiV1LidToAkash(mani.Spec.LeaseID)})
				if err != nil {
					return err
				}
				if lease.Lease.State == mtypes.LeaseClosed {
					fmt.Printf("pruning \"%s\" manifest\n", mani.Name)
					_ = ac.AkashV1().Manifests(ns).Delete(ctx, mani.Name, metav1.DeleteOptions{})
					oldMani = append(oldMani[:i], oldMani[i+1:]...)
				}
			}

			if len(mList.Items) > 0 {
				_ = os.MkdirAll(manifestsPath, 0755)
			}

			if len(hList.Items) > 0 {
				_ = os.MkdirAll(hostsPath, 0755)
			}

			// backup manifests
			fmt.Println("backup manifests")
			for i := range oldMani {
				data, _ := json.MarshalIndent(&oldMani[i], "", "  ")
				if err = backupObject(manifestsPath+"/"+oldMani[i].Name+".yaml", data); err != nil {
					return err
				}
				_ = ac.AkashV1().Manifests(ns).Delete(ctx, oldMani[i].Name, metav1.DeleteOptions{})
			}
			fmt.Println("backup manifests DONE")

			fmt.Println("backup provider hosts")
			for i := range oldHosts {
				data, _ := json.MarshalIndent(&oldHosts[i], "", "  ")
				if err = backupObject(hostsPath+"/"+oldHosts[i].Name+".yaml", data); err != nil {
					return err
				}

				_ = ac.AkashV1().ProviderHosts(ns).Delete(ctx, oldHosts[i].Name, metav1.DeleteOptions{})
			}
			fmt.Println("backup provider hosts DONE")
		}
	}

	if len(oldMani) == 0 {
		oldMani, err = loadManifests(manifestsPath)
		if err != nil {
			return err
		}
	}

	if len(oldHosts) == 0 {
		oldHosts, err = loadHosts(hostsPath)
		if err != nil {
			return err
		}
	}

	fmt.Println("applying manifests")
	for _, mani := range oldMani {

		nmani := &v2beta1.Manifest{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Manifest",
				APIVersion: "akash.network/v2beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      mani.Name,
				Namespace: ns,
			},
		}

		nmani.Labels = mani.Labels
		nmani.Spec = v2beta1migrate.ManifestSpecFromV1(mani.Spec)

		// double check this manifest not present in the new api
		_, err = ac.AkashV2beta1().Manifests(ns).Get(ctx, mani.Name, metav1.GetOptions{})
		if err != nil && !kubeErrors.IsNotFound(err) {
			fmt.Printf("unable to check presence of \"%s\" manifest. still trying to migrate. %s\n", mani.Name, err.Error())
		}

		_, err = ac.AkashV2beta1().Manifests(ns).Create(ctx, nmani, metav1.CreateOptions{})
		if err == nil {
			fmt.Printf("manifest \"%s\" has been migrated successfully\n", mani.Name)
		} else {
			fmt.Printf("manifest \"%s\" migration failed. error: %s\n", mani.Name, err.Error())
		}
	}
	fmt.Println("applying manifests  DONE")

	fmt.Println("applying hosts")
	for _, host := range oldHosts {
		nhost := &v2beta1.ProviderHost{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Manifest",
				APIVersion: "akash.network/v2beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      host.Name,
				Namespace: ns,
			},
		}

		nhost.Labels = host.Labels
		nhost.Spec = v2beta1migrate.ProviderHostsSpecFromV1(host.Spec)

		// double check this manifest not present in the new api
		_, err = ac.AkashV2beta1().ProviderHosts(ns).Get(ctx, host.Name, metav1.GetOptions{})
		if err != nil && !kubeErrors.IsNotFound(err) {
			fmt.Printf("unable to check presence of \"%s\" manifest. still trying to migrate. %s\n", host.Name, err.Error())
		}

		_, err = ac.AkashV2beta1().ProviderHosts(ns).Create(ctx, nhost, metav1.CreateOptions{})
		if err == nil {
			fmt.Printf("provider host \"%s\" has been migrated successfully\n", host.Name)
		} else {
			fmt.Printf("provider host \"%s\" migration failed. error: %s\n", host.Name, err.Error())
		}
	}
	fmt.Println("applying hosts      DONE")

	return nil
}

func openKubeConfig(cfgPath string) (*rest.Config, error) {
	// If no value is specified, use a default
	if len(cfgPath) == 0 {
		cfgPath = path.Join(homedir.HomeDir(), ".kube", "config")
	}

	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("using kube config file %s\n", cfgPath)
		return clientcmd.BuildConfigFromFlags("", cfgPath)
	}

	fmt.Println("using in cluster kube config")
	return rest.InClusterConfig()
}

func apiV1LidToAkash(from v1.LeaseID) mtypes.LeaseID {
	// any chance of this conversion to fail?
	owner, _ := sdk.AccAddressFromBech32(from.Owner)
	provider, _ := sdk.AccAddressFromBech32(from.Provider)
	dseq, _ := strconv.ParseUint(from.DSeq, 10, 64)

	return mtypes.LeaseID{
		Owner:    owner.String(),
		DSeq:     dseq,
		GSeq:     from.GSeq,
		OSeq:     from.OSeq,
		Provider: provider.String(),
	}
}

func isDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = f.Close()
	}()

	// read in ONLY one file
	_, err = f.Readdir(1)

	// and if the file is EOF... well, the dir is empty.
	if err == io.EOF {
		return true, nil
	}

	return false, nil
}

func backupObject(path string, data []byte) error {
	fl, err := os.Create(path)
	if err != nil {
		return err
	}

	defer func() {
		_ = fl.Close()
	}()

	_, err = fl.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func loadManifests(path string) ([]v1.Manifest, error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var res []v1.Manifest

	for _, fl := range files {
		if fl.IsDir() || !strings.HasSuffix(fl.Name(), ".yaml") {
			continue
		}

		obj := v1.Manifest{}

		if err = readObject(path+"/"+fl.Name(), &obj); err == nil {
			res = append(res, obj)
		} else {
			fmt.Printf("error reading manifest from \"%s\". %s", fl.Name(), err.Error())
		}
	}

	return res, nil
}

func loadHosts(path string) ([]v1.ProviderHost, error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var res []v1.ProviderHost

	for _, fl := range files {
		if fl.IsDir() || !strings.HasSuffix(fl.Name(), ".yaml") {
			continue
		}

		obj := v1.ProviderHost{}

		if err = readObject(path+"/"+fl.Name(), &obj); err == nil {
			res = append(res, obj)
		} else {
			fmt.Printf("error reading manifest from \"%s\". %s", fl.Name(), err.Error())
		}
	}

	return res, nil
}

func readObject(path string, obj interface{}) error {
	fl, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = fl.Close()
	}()

	data, err := ioutil.ReadAll(fl)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(data, &obj); err != nil {
		return err
	}

	return nil
}
