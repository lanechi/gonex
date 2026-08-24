package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanechi/gonex/gx/internal/gen"
	"github.com/spf13/cobra"
)

// Run executes the gx command line without calling os.Exit, which keeps the
// command easy to test and safe to embed in another developer tool.
func Run(args []string) error {
	command := newRootCommand()
	command.SetArgs(args)
	command.SetOut(os.Stdout)
	command.SetErr(os.Stderr)
	return command.Execute()
}

func newRootCommand() *cobra.Command {
	command := &cobra.Command{
		Use:           "gx",
		Short:         "生成项目代码",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newInitCommand(), newDAOCommand(), newCtrlCommand(), newServiceCommand())
	command.SetHelpTemplate(`{{with (or .Long .Short)}}{{.}}{{end}}

用法:
  {{.UseLine}}

{{if .HasAvailableSubCommands}}命令:
{{range .Commands}}{{if .IsAvailableCommand}}  {{rpad .Name .NamePadding }} {{.Short}}{{"\n"}}{{end}}{{end}}{{end}}{{if .HasAvailableFlags}}
选项:
{{.LocalNonPersistentFlags.FlagUsages | trimRightSpace}}{{end}}
`)
	return command
}

func newInitCommand() *cobra.Command {
	var module string
	var name string
	var templateURL string
	var force bool
	var dryRun bool
	command := &cobra.Command{
		Use:   "init [path]",
		Short: "从 canonical demo 初始化 PostgreSQL GORM 项目",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			targetArg := "."
			if len(args) == 1 {
				targetArg = args[0]
			}
			target, err := filepath.Abs(targetArg)
			if err != nil {
				return fmt.Errorf("resolve init path: %w", err)
			}
			projectModule := strings.TrimSpace(module)
			if projectModule == "" {
				projectModule = filepath.ToSlash(filepath.Base(target))
			}
			result, err := gen.InitProject(target, gen.InitOptions{
				ModulePath:  projectModule,
				Name:        name,
				TemplateURL: templateURL,
				Force:       force,
				DryRun:      dryRun,
			})
			printResult(command.OutOrStdout(), result)
			return err
		},
	}
	command.Flags().StringVar(&module, "module", "", "Go module 路径，默认使用项目名称")
	command.Flags().StringVar(&name, "name", "", "项目名称，默认使用目标目录名")
	command.Flags().StringVar(&templateURL, "template-url", "", "模板 archive URL（用于镜像或测试）")
	command.Flags().BoolVar(&force, "force", false, "允许在非空目录初始化")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "只列出下载和创建计划，不联网或写入文件")
	return command
}

func newDAOCommand() *cobra.Command {
	var tables string
	command := &cobra.Command{
		Use:   "dao",
		Short: "根据数据库生成 GORM DAO 和实体",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			project, err := gen.DiscoverProject(".")
			if err != nil {
				return err
			}
			result, err := gen.GenerateModels(project, gen.ModelOptions{
				Tables: tables,
			})
			printResult(command.OutOrStdout(), result)
			return err
		},
	}
	command.Flags().StringVar(&tables, "tables", "", "逗号分隔的数据表名称；PostgreSQL 支持 schema.table")
	return command
}

func newCtrlCommand() *cobra.Command {
	var dryRun bool
	var clean bool
	var directory string
	command := &cobra.Command{
		Use:   "ctrl [name[,name...]]",
		Short: "从 API 定义生成 Controller",
		Long:  "从 API 定义生成 Controller；名称可为 user，也可为 user/v1/newapi 这样的 API 路径，多个名称用逗号分隔。",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			apiDirectory, err := normalizeAPIDirectory(directory)
			if err != nil {
				return err
			}
			project, err := gen.DiscoverProject(".")
			if err != nil {
				return err
			}
			names, err := splitNames(args)
			if err != nil {
				return err
			}
			var combined gen.Result
			if len(names) == 0 {
				result, generateErr := gen.GenerateControllers(project, gen.ControllerOptions{
					Source: apiDirectory,
					DryRun: dryRun,
					Clean:  clean,
				})
				combined.Changes = append(combined.Changes, result.Changes...)
				printResult(command.OutOrStdout(), combined)
				return generateErr
			}
			for _, name := range names {
				result, generateErr := gen.GenerateControllers(project, gen.ControllerOptions{
					Source: apiDirectory,
					Name:   name,
					DryRun: dryRun,
					Clean:  clean,
				})
				combined.Changes = append(combined.Changes, result.Changes...)
				if generateErr != nil {
					printResult(command.OutOrStdout(), combined)
					return generateErr
				}
			}
			printResult(command.OutOrStdout(), combined)
			return nil
		},
	}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "只输出生成计划，不写入文件")
	command.Flags().BoolVar(&clean, "clean", false, "删除已失效的 gx 生成契约文件")
	command.Flags().StringVar(&directory, "dir", "", "API 目录（相对默认 api 目录），例如 /user/v1")
	return command
}

func newServiceCommand() *cobra.Command {
	var module string
	var dryRun bool
	command := &cobra.Command{
		Use:   "service [name[,name...]]",
		Short: "从 Logic 定义生成 Service",
		Long:  "从 Logic 定义生成 Service；多个名称用逗号分隔。",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			names, err := splitNames(args)
			if err != nil {
				return err
			}
			if len(names) > 0 && strings.TrimSpace(module) != "" {
				return fmt.Errorf("service 不能同时使用名称和 --module")
			}
			project, err := gen.DiscoverProject(".")
			if err != nil {
				return err
			}
			var combined gen.Result
			if len(names) == 0 {
				result, generateErr := gen.GenerateServices(project, gen.ServiceOptions{
					Module: module,
					DryRun: dryRun,
				})
				combined.Changes = append(combined.Changes, result.Changes...)
				printResult(command.OutOrStdout(), combined)
				return generateErr
			}
			for _, name := range names {
				result, generateErr := gen.GenerateServices(project, gen.ServiceOptions{
					Name:   name,
					DryRun: dryRun,
				})
				combined.Changes = append(combined.Changes, result.Changes...)
				if generateErr != nil {
					printResult(command.OutOrStdout(), combined)
					return generateErr
				}
			}
			printResult(command.OutOrStdout(), combined)
			return nil
		},
	}
	command.Flags().StringVar(&module, "module", "", "按已有 Logic 模块筛选生成")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "只输出生成计划，不写入文件")
	return command
}

func splitNames(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	parts := strings.Split(args[0], ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("名称列表不能包含空名称")
		}
		names = append(names, part)
	}
	return names, nil
}

func normalizeAPIDirectory(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.Trim(value, "/")
	if value == "" {
		return "", fmt.Errorf("--dir 不能是根目录")
	}
	if value == "api" || strings.HasPrefix(value, "api/") {
		return filepath.ToSlash(value), nil
	}
	return filepath.ToSlash(filepath.Join("api", value)), nil
}

func printResult(writer io.Writer, result gen.Result) {
	for _, change := range result.Changes {
		if change.Detail == "" {
			fmt.Fprintf(writer, "%-7s %s\n", change.Kind, change.Path)
			continue
		}
		fmt.Fprintf(writer, "%-7s %s: %s\n", change.Kind, change.Path, change.Detail)
	}
}
