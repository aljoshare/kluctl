package commands

import (
	"fmt"
	"github.com/codablock/kluctl/cmd/kluctl/args"
	"github.com/codablock/kluctl/pkg/kluctl_project"
	"github.com/codablock/kluctl/pkg/seal"
	"github.com/codablock/kluctl/pkg/types"
	"github.com/codablock/kluctl/pkg/utils/uo"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	SecretsDir  string
	ForceReseal bool
)

func findSecretsEntry(ctx *commandCtx, name string) (*types.SecretSet, error) {
	for _, e := range ctx.kluctlProject.Config.SecretsConfig.SecretSets {
		if e.Name == name {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("secret Set with name %s was not found", name)
}

func loadSecrets(ctx *commandCtx, target *types.Target, secretsLoader *seal.SecretsLoader) error {
	secrets := uo.New()
	for _, secretSetName := range target.SealingConfig.SecretSets {
		secretEntry, err := findSecretsEntry(ctx, secretSetName)
		if err != nil {
			return err
		}
		for _, source := range secretEntry.Sources {
			var renderedSource types.SecretSource
			err = ctx.kluctlProject.J2.RenderStruct(&renderedSource, &source, ctx.deploymentProject.VarsCtx.Vars)
			if err != nil {
				return err
			}
			s, err := secretsLoader.LoadSecrets(&renderedSource)
			if err != nil {
				return err
			}
			secrets.Merge(s)
		}
	}
	ctx.deploymentProject.MergeSecretsIntoAllChildren(secrets)
	return nil
}

func runCmdSealForTarget(p *kluctl_project.KluctlProjectContext, target *types.Target, secretsLoader *seal.SecretsLoader) error {
	log.Infof("Sealing for target %s", target.Name)

	// pass forSeal=True so that .sealme files are rendered as well
	return withProjectTargetCommandContext(p, target, true, func(ctx *commandCtx) error {
		err := loadSecrets(ctx, target, secretsLoader)
		if err != nil {
			return err
		}
		err = ctx.deploymentCollection.RenderDeployments(ctx.k)
		if err != nil {
			return err
		}

		sealedSecretsNamespace := "kube-system"
		sealedSecretsControllerName := "sealed-secrets-controller"
		if p.Config.SecretsConfig != nil && p.Config.SecretsConfig.SealedSecrets != nil {
			if p.Config.SecretsConfig.SealedSecrets.Namespace != nil {
				sealedSecretsNamespace = *p.Config.SecretsConfig.SealedSecrets.Namespace
			}
			if p.Config.SecretsConfig.SealedSecrets.ControllerName != nil {
				sealedSecretsControllerName = *p.Config.SecretsConfig.SealedSecrets.ControllerName
			}
		}
		if p.Config.SecretsConfig == nil || p.Config.SecretsConfig.SealedSecrets == nil || p.Config.SecretsConfig.SealedSecrets.Bootstrap == nil || *p.Config.SecretsConfig.SealedSecrets.Bootstrap {
			err = seal.BootstrapSealedSecrets(ctx.k, sealedSecretsNamespace)
			if err != nil {
				return err
			}
		}

		clusterConfig, err := p.LoadClusterConfig(target.Cluster)
		if err != nil {
			return err
		}
		sealer, err := seal.NewSealer(ctx.k, sealedSecretsNamespace, sealedSecretsControllerName, clusterConfig.Cluster, ForceReseal)
		if err != nil {
			return err
		}

		err = ctx.deploymentCollection.Seal(sealer)
		if err != nil {
			return err
		}
		return err
	})
}

func runCmdSeal(cmd *cobra.Command, args_ []string) error {
	return withKluctlProjectFromArgs(func(p *kluctl_project.KluctlProjectContext) error {
		hadError := false

		secretsLoader := seal.NewSecretsLoader(p, SecretsDir)

		baseTargets := make(map[string]bool)
		noTargetMatch := true
		for _, target := range p.DynamicTargets {
			if args.Target != "" && args.Target != target.Target.Name {
				continue
			}
			if args.Cluster != "" && args.Cluster != target.Target.Cluster {
				continue
			}
			if target.Target.SealingConfig == nil {
				log.Infof("Target %s has no sealingConfig", target.Target.Name)
				continue
			}
			noTargetMatch = false

			sealTarget := target.Target
			dynamicSealing := target.Target.SealingConfig.DynamicSealing == nil || *target.Target.SealingConfig.DynamicSealing
			isDynamicTarget := target.BaseTargetName != target.Target.Name
			if !dynamicSealing && isDynamicTarget {
				baseTarget, err := p.FindBaseTarget(target.BaseTargetName)
				if err != nil {
					return err
				}
				if baseTargets[target.BaseTargetName] {
					// Skip this target as it was already sealed
					continue
				}
				baseTargets[target.BaseTargetName] = true
				sealTarget = baseTarget
			}

			err := runCmdSealForTarget(p, sealTarget, secretsLoader)
			if err != nil {
				log.Warningf("Sealing for target %s failed: %v", sealTarget.Name, err)
				hadError = true
			}
		}
		if hadError {
			return fmt.Errorf("sealing for at least one target failed")
		}
		if noTargetMatch {
			return fmt.Errorf("no target matched the given target and/or cluster name")
		}
		return nil
	})
}

func init() {
	cmd := &cobra.Command{
		Use:   "seal",
		Short: "Seal secrets based on target's sealingConfig",
		Long: "Loads all secrets from the specified secrets sets from the target's sealingConfig and " +
			"then renders the target, including all files with the `.sealme` extension. Then runs " +
			"kubeseal on each `.sealme` file and stores secrets in the directory specified by " +
			"`--local-sealed-secrets`, using the outputPattern from your deployment project.\n\n" +
			"If no `--target` is specified, sealing is performed for all targets.",
		RunE: runCmdSeal,
	}

	args.AddProjectArgs(cmd, true, true, true)

	cmd.Flags().StringVar(&SecretsDir, "secrets-dir", ".", "Specifies where to find unencrypted secret files. The given directory is NOT meant to be part "+
		"of your source repository! The given path only matters for secrets of type 'path'. Defaults "+
		"to the current working directory.")
	cmd.Flags().BoolVar(&ForceReseal, "force-reseal", false, "Lets kluctl ignore secret hashes found in already sealed secrets and thus forces resealing of those.")

	err := viper.BindPFlags(cmd.Flags())
	if err != nil {
		panic(err)
	}

	rootCmd.AddCommand(cmd)
}