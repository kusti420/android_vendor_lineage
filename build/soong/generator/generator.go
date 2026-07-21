// Copyright 2015 Google Inc. All rights reserved.
// Copyright (C) 2018,2021 The LineageOS Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generator

import (
	"fmt"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"path/filepath"
)

func init() {
	android.RegisterModuleType("lineage_generator", GeneratorFactory)
}

var String = proptools.String

var (
	pctx = android.NewPackageContext("android/soong/generator")
)

type HostToolProvider interface {
	HostToolPath() android.OptionalPath
}

type hostToolDependencyTag struct {
	blueprint.BaseDependencyTag
}

var hostToolDepTag hostToolDependencyTag

type generatorProperties struct {
	// The command to run on one or more input files. Cmd supports substitution of a few variables
	// (the actual substitution is implemented in GenerateAndroidBuildActions below)
	//
	// Available variables for substitution:
	//
	//  $(location): the path to the first entry in tools or tool_files
	//  $(location <label>): the path to the tool or tool_file with name <label>
	//  $(genDir): the sandbox directory for this tool; contains $(out)
	//  $$: a literal $
	//
	Cmd *string

	// name of the modules (if any) that produces the host executable.   Leave empty for
	// prebuilts or scripts that do not need a module to build them.
	Tools []string

	// Local file that is used as the tool
	Tool_files []string

	// List of directories to export as headers
	Export_include_dirs []string

	// List of directories to export as sources
	Export_source_dirs []string

	// Root directory for dep_files.
	// Relative to top build dir.
	Dep_root *string

	// Declare list of files that should be used for timestamp dependency checking
	Dep_files []string
}

type Module struct {
	android.ModuleBase

	properties generatorProperties

	rule blueprint.Rule

	// Tool dependencies
	implicitDeps android.Paths
	// Deps from input files declared in dep_files property.
	inputDeps android.Paths

	exportedIncludeDirs android.Paths
	exportedSourceDirs  android.Paths

	outputDeps android.Paths
}

// These three methods satisfy genrule.SourceFileGenerator.
// Which cc modules check for when including headers etc.
func (g *Module) GeneratedHeaderDirs() android.Paths {
	return g.exportedIncludeDirs
}

func (g *Module) GeneratedSourceFiles() android.Paths {
	return g.exportedSourceDirs
}

func (g *Module) GeneratedDeps() android.Paths {
	return g.outputDeps
}

func (g *Module) DepsMutator(ctx android.BottomUpMutatorContext) {
	android.ExtractSourcesDeps(ctx, g.properties.Dep_files)
	android.ExtractSourcesDeps(ctx, g.properties.Tool_files)
	if g, ok := ctx.Module().(*Module); ok {
		if len(g.properties.Tools) > 0 {
			ctx.AddFarVariationDependencies(ctx.Config().BuildOSTarget.Variations(),
				hostToolDepTag, g.properties.Tools...)
		}
	}
}

func (g *Module) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(g.properties.Export_include_dirs) > 0 {
		for _, dir := range g.properties.Export_include_dirs {
			g.exportedIncludeDirs = append(g.exportedIncludeDirs,
				android.PathForModuleGen(ctx, dir))
		}
	} else {
		g.exportedIncludeDirs = append(g.exportedIncludeDirs, android.PathForModuleGen(ctx, ""))
	}
	if len(g.properties.Export_source_dirs) > 0 {
		for _, dir := range g.properties.Export_source_dirs {
			g.exportedSourceDirs = append(g.exportedSourceDirs,
				android.PathForModuleGen(ctx, dir))
		}
	} else {
		g.exportedSourceDirs = append(g.exportedSourceDirs, android.PathForModuleGen(ctx, ""))
	}

	tools := map[string]android.Path{}

	if len(g.properties.Tools) > 0 {
		// IchthysOS/AOSP-17: the tools-based dependency path used soong provider APIs
		// (android.PrebuiltGetPreferred / HostToolProviderInfoProvider / CommonModuleInfoProvider)
		// that are not present in this tree's build/soong. No lineage_generator in this tree uses
		// the `tools` property (the kernel-header generators use cmd/tool_files only), so this
		// path is intentionally unsupported here rather than ported.
		ctx.ModuleErrorf("lineage_generator: 'tools' property is unsupported in this build; use tool_files")
	}

	if ctx.Failed() {
		return
	}

	toolFiles := android.PathsForModuleSrc(ctx, g.properties.Tool_files)
	for _, tool := range toolFiles {
		g.implicitDeps = append(g.implicitDeps, tool)
		if _, exists := tools[tool.Rel()]; !exists {
			tools[tool.Rel()] = tool
		} else {
			ctx.ModuleErrorf("multiple tools for %q, %q and %q", tool, tools[tool.Rel()], tool.Rel())
		}
	}

	// Determine root dir for dep_files.  Defaults to current ctx ModuleDir.
	depRoot := String(g.properties.Dep_root)
	if depRoot == "" {
		depRoot = ctx.ModuleDir()
	} else {
		depRoot = lineageExpandVariables(ctx, depRoot)
	}

	// Glob dep_files property
	for _, dep_file := range g.properties.Dep_files {
		dep_file = lineageExpandVariables(ctx, dep_file)
		globPath := filepath.Join(depRoot, dep_file)
		paths, err := ctx.GlobWithDeps(globPath, nil)
		if err != nil {
			ctx.ModuleErrorf("unable to glob %s: %s", globPath, err.Error())
			return
		}
		for _, path := range paths {
			g.inputDeps = append(g.inputDeps, android.PathForSourceRelaxed(ctx, path))
		}
	}

	cmd := lineageExpandVariables(ctx, String(g.properties.Cmd))

	rawCommand, err := android.Expand(cmd, func(name string) (string, error) {
		switch name {
		case "location":
			if len(g.properties.Tools) == 0 && len(toolFiles) == 0 {
				return "", fmt.Errorf("at least one `tools` or `tool_files` is required if $(location) is used")
			}

			if len(g.properties.Tools) > 0 {
				return tools[g.properties.Tools[0]].String(), nil
			} else {
				return tools[toolFiles[0].Rel()].String(), nil
			}
		case "genDir":
			return android.PathForModuleGen(ctx).String(), nil
		default:
			if strings.HasPrefix(name, "location ") {
				label := strings.TrimSpace(strings.TrimPrefix(name, "location "))
				if tool, ok := tools[label]; ok {
					return tool.String(), nil
				} else {
					return "", fmt.Errorf("unknown location label %q", label)
				}
			}
			// IchthysOS: treat an unknown variable as empty (like make does for an unset
			// var) instead of erroring. On products that don't build a kernel (e.g.
			// Cuttlefish, TARGET_NO_KERNEL_OVERRIDE), the KERNEL_* vars used by the
			// generated_kernel_includes/prebuilt_kernel_includes cmds are never set, so the
			// old hard error broke the whole build even though those modules are never built.
			return "", nil
		}
	})

	if err != nil {
		ctx.PropertyErrorf("cmd", "%s", err.Error())
		return
	}

	// Dummy output dep
	dummyDep := android.PathForModuleGen(ctx, ".dummy_dep")

	genDir := android.PathForModuleGen(ctx)
	_ = genDir

	// IchthysOS/AOSP-17: run UNSANDBOXED. The kernel headers_install is an in-tree operation and
	// the sbox sandbox fails copying the kernel's arch/<arch>/include/asm symlink-dir
	// ("copy_file_range: is a directory"). Without Sbox the command runs from the tree root and
	// writes generated UAPI headers directly into $(genDir).
	rule := android.NewRuleBuilder(pctx, ctx)

	rule.Command().
		Text(rawCommand).
		ImplicitOutput(dummyDep).
		Implicits(g.inputDeps).
		Implicits(g.implicitDeps)

	rule.Command().Text("touch").Output(dummyDep)

	g.outputDeps = append(g.outputDeps, dummyDep)

	rule.Build("generator", "generate")
}

func NewGenerator() *Module {
	module := &Module{}
	module.AddProperties(&module.properties)
	return module
}

func GeneratorFactory() android.Module {
	m := NewGenerator()
	android.InitAndroidModule(m)
	return m
}
