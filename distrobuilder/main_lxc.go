package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lxc/distrobuilder/generators"
	"github.com/lxc/distrobuilder/image"
	"github.com/lxc/distrobuilder/managers"
	"github.com/lxc/distrobuilder/shared"
)

type cmdLXC struct {
	cmdBuild *cobra.Command
	cmdPack  *cobra.Command
	global   *cmdGlobal

	flagCompression string
}

func (c *cmdLXC) commandBuild() *cobra.Command {
	c.cmdBuild = &cobra.Command{
		Use:   "build-lxc <filename|-> [target dir] [--compression=COMPRESSION]",
		Short: "Build LXC image from scratch",
		Long: fmt.Sprintf(`Build LXC image from scratch

%s
`, compressionDescription),
		Args:    cobra.RangeArgs(1, 2),
		PreRunE: c.global.preRunBuild,
		RunE: func(cmd *cobra.Command, args []string) error {
			overlayDir, cleanup, err := c.global.getOverlayDir()
			if err != nil {
				return fmt.Errorf("Failed to get overlay directory: %w", err)
			}

			if cleanup != nil {
				c.global.overlayCleanup = cleanup

				defer func() {
					cleanup()
					c.global.overlayCleanup = nil
				}()
			}

			return c.run(cmd, args, overlayDir)
		},
	}

	c.cmdBuild.Flags().StringVar(&c.flagCompression, "compression", "xz", "Type of compression to use"+"``")

	return c.cmdBuild
}

func (c *cmdLXC) commandPack() *cobra.Command {
	c.cmdPack = &cobra.Command{
		Use:   "pack-lxc <filename|-> <source dir> [target dir] [--compression=COMPRESSION]",
		Short: "Create LXC image from existing rootfs",
		Long: fmt.Sprintf(`Create LXC image from existing rootfs

%s
`, compressionDescription),
		Args:    cobra.RangeArgs(2, 3),
		PreRunE: c.global.preRunPack,
		RunE: func(cmd *cobra.Command, args []string) error {
			overlayDir, cleanup, err := c.global.getOverlayDir()
			if err != nil {
				return fmt.Errorf("Failed to get overlay directory: %w", err)
			}

			if cleanup != nil {
				c.global.overlayCleanup = cleanup

				defer func() {
					cleanup()
					c.global.overlayCleanup = nil
				}()
			}

			err = c.runPack(cmd, args, overlayDir)
			if err != nil {
				return fmt.Errorf("Failed to pack image: %w", err)
			}

			return c.run(cmd, args, overlayDir)
		},
	}

	c.cmdPack.Flags().StringVar(&c.flagCompression, "compression", "xz", "Type of compression to use"+"``")

	return c.cmdPack
}

func (c *cmdLXC) runPack(cmd *cobra.Command, args []string, overlayDir string) error {
	// Setup the mounts and chroot into the rootfs
	exitChroot, err := shared.SetupChroot(overlayDir, c.global.definition.Environment, nil)
	if err != nil {
		return fmt.Errorf("Failed to setup chroot: %w", err)
	}
	// Unmount everything and exit the chroot
	defer exitChroot()

	imageTargets := shared.ImageTargetAll | shared.ImageTargetContainer

	manager, err := managers.Load(c.global.definition.Packages.Manager, c.global.logger, *c.global.definition)
	if err != nil {
		return fmt.Errorf("Failed to load manager %q: %w", c.global.definition.Packages.Manager, err)
	}

	c.global.logger.Info("Managing repositories")

	err = manager.ManageRepositories(imageTargets)
	if err != nil {
		return fmt.Errorf("Failed to manage repositories: %w", err)
	}

	c.global.logger.Infow("Running hooks", "trigger", "post-unpack")

	// Run post unpack hook
	for _, hook := range c.global.definition.GetRunnableActions("post-unpack", imageTargets) {
		err := shared.RunScript(hook.Action)
		if err != nil {
			return fmt.Errorf("Failed to run post-unpack: %w", err)
		}
	}

	c.global.logger.Info("Managing packages")

	// Install/remove/update packages
	err = manager.ManagePackages(imageTargets)
	if err != nil {
		return fmt.Errorf("Failed to manage packages: %w", err)
	}

	c.global.logger.Infow("Running hooks", "trigger", "post-packages")

	// Run post packages hook
	for _, hook := range c.global.definition.GetRunnableActions("post-packages", imageTargets) {
		err := shared.RunScript(hook.Action)
		if err != nil {
			return fmt.Errorf("Failed to run post-packages: %w", err)
		}
	}

	return nil
}

func (c *cmdLXC) run(cmd *cobra.Command, args []string, overlayDir string) error {
	img := image.NewLXCImage(overlayDir, c.global.targetDir,
		c.global.flagCacheDir, *c.global.definition)

	for _, file := range c.global.definition.Files {
		if !shared.ApplyFilter(&file, c.global.definition.Image.Release, c.global.definition.Image.ArchitectureMapped, c.global.definition.Image.Variant, c.global.definition.Targets.Type, shared.ImageTargetUndefined|shared.ImageTargetAll|shared.ImageTargetContainer) {
			c.global.logger.Infow("Skipping generator", "generator", file.Generator)
			continue
		}

		generator, err := generators.Load(file.Generator, c.global.logger, c.global.flagCacheDir, overlayDir, file, *c.global.definition)
		if err != nil {
			return fmt.Errorf("Failed to load generator %q: %w", file.Generator, err)
		}

		c.global.logger.Infow("Running generator", "generator", file.Generator)

		err = generator.RunLXC(img, c.global.definition.Targets.LXC)
		if err != nil {
			return fmt.Errorf("Failed to run generator %q: %w", file.Generator, err)
		}
	}

	exitChroot, err := shared.SetupChroot(overlayDir,
		c.global.definition.Environment, nil)
	if err != nil {
		return fmt.Errorf("Failed to setup chroot in %q: %w", overlayDir, err)
	}

	addSystemdGenerator()

	c.global.logger.Infow("Running hooks", "trigger", "post-files")

	// Run post files hook
	for _, action := range c.global.definition.GetRunnableActions("post-files", shared.ImageTargetUndefined|shared.ImageTargetAll|shared.ImageTargetContainer) {
		err := shared.RunScript(action.Action)
		if err != nil {
			exitChroot()
			return fmt.Errorf("Failed to run post-files: %w", err)
		}
	}

	exitChroot()

	c.global.logger.Infow("Creating LXC image", "compression", c.flagCompression)

	err = img.Build(c.flagCompression)
	if err != nil {
		return fmt.Errorf("Failed to create LXC image: %w", err)
	}

	return nil
}
