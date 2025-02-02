package module_manager

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kennygrant/sanitize"
	"github.com/romana/rlog"

	"github.com/flant/shell-operator/pkg/executor"
	"github.com/flant/shell-operator/pkg/kube_events_manager"
	"github.com/flant/shell-operator/pkg/schedule_manager"
	utils_data "github.com/flant/shell-operator/pkg/utils/data"

	"github.com/flant/addon-operator/pkg/helm"
	"github.com/flant/addon-operator/pkg/utils"
)

type GlobalHook struct {
	*CommonHook
	Config *GlobalHookConfig
}

type ModuleHook struct {
	*CommonHook
	Module *Module
	Config *ModuleHookConfig
}

type Hook interface {
	GetName() string
	GetPath() string
	PrepareTmpFilesForHookRun(context []BindingContext) (map[string]string, error)
}

type CommonHook struct {
	// The unique name like 'global-hooks/startup_hook' or '002-module/hooks/cleanup'.
	Name           string
	// The absolute path of the executable file.
	Path           string

	Bindings       []BindingType
	OrderByBinding map[BindingType]float64

	moduleManager *MainModuleManager
}

type GlobalHookConfig struct {
	HookConfig
	BeforeAll interface{} `json:"beforeAll"`
	AfterAll  interface{} `json:"afterAll"`
}

type ModuleHookConfig struct {
	HookConfig
	BeforeHelm      interface{} `json:"beforeHelm"`
	AfterHelm       interface{} `json:"afterHelm"`
	AfterDeleteHelm interface{} `json:"afterDeleteHelm"`
}

type HookConfig struct {
	OnStartup         interface{}                                   `json:"onStartup"`
	Schedule          []schedule_manager.ScheduleConfig             `json:"schedule"`
	OnKubernetesEvent []kube_events_manager.OnKubernetesEventConfig `json:"onKubernetesEvent"`
}

func NewGlobalHook(name, path string, config *GlobalHookConfig, mm *MainModuleManager) *GlobalHook {
	globalHook := &GlobalHook{}
	globalHook.CommonHook = NewHook(name, path, mm)
	globalHook.Config = config
	return globalHook
}

func NewHook(name, path string, mm *MainModuleManager) *CommonHook {
	hook := &CommonHook{}
	hook.moduleManager = mm
	hook.Name = name
	hook.Path = path
	hook.OrderByBinding = make(map[BindingType]float64)
	return hook
}

func NewModuleHook(name, path string, config *ModuleHookConfig, mm *MainModuleManager) *ModuleHook {
	moduleHook := &ModuleHook{}
	moduleHook.CommonHook = NewHook(name, path, mm)
	moduleHook.Config = config
	return moduleHook
}

func (mm *MainModuleManager) registerGlobalHook(name, path string, config *GlobalHookConfig) (err error) {
	var ok bool
	globalHook := NewGlobalHook(name, path, config, mm)

	if config.BeforeAll != nil {
		globalHook.Bindings = append(globalHook.Bindings, BeforeAll)
		if globalHook.OrderByBinding[BeforeAll], ok = config.BeforeAll.(float64); !ok {
			return fmt.Errorf("unsuported value '%v' for binding '%s'", config.BeforeAll, BeforeAll)
		}
		mm.globalHooksOrder[BeforeAll] = append(mm.globalHooksOrder[BeforeAll], globalHook)
	}

	if config.AfterAll != nil {
		globalHook.Bindings = append(globalHook.Bindings, AfterAll)
		if globalHook.OrderByBinding[AfterAll], ok = config.AfterAll.(float64); !ok {
			return fmt.Errorf("unsuported value '%v' for binding '%s'", config.AfterAll, AfterAll)
		}
		mm.globalHooksOrder[AfterAll] = append(mm.globalHooksOrder[AfterAll], globalHook)
	}

	if config.OnStartup != nil {
		globalHook.Bindings = append(globalHook.Bindings, OnStartup)
		if globalHook.OrderByBinding[OnStartup], ok = config.OnStartup.(float64); !ok {
			return fmt.Errorf("unsuported value '%v' for binding '%s'", config.OnStartup, OnStartup)
		}
		mm.globalHooksOrder[OnStartup] = append(mm.globalHooksOrder[OnStartup], globalHook)
	}

	if len(config.Schedule) != 0 {
		globalHook.Bindings = append(globalHook.Bindings, Schedule)
		mm.globalHooksOrder[Schedule] = append(mm.globalHooksOrder[Schedule], globalHook)
	}

	if len(config.OnKubernetesEvent) != 0 {
		globalHook.Bindings = append(globalHook.Bindings, KubeEvents)
		mm.globalHooksOrder[KubeEvents] = append(mm.globalHooksOrder[KubeEvents], globalHook)
	}

	mm.globalHooksByName[name] = globalHook

	return nil
}

func (mm *MainModuleManager) registerModuleHook(moduleName, name, path string, config *ModuleHookConfig) (err error) {
	var ok bool
	moduleHook := NewModuleHook(name, path, config, mm)

	if moduleHook.Module, err = mm.GetModule(moduleName); err != nil {
		return err
	}

	if config.BeforeHelm != nil {
		moduleHook.Bindings = append(moduleHook.Bindings, BeforeHelm)
		if moduleHook.OrderByBinding[BeforeHelm], ok = config.BeforeHelm.(float64); !ok {
			return fmt.Errorf("unsuported value '%v' for binding '%s'", config.BeforeHelm, BeforeHelm)
		}

		mm.addModulesHooksOrderByName(moduleName, BeforeHelm, moduleHook)
	}

	if config.AfterHelm != nil {
		moduleHook.Bindings = append(moduleHook.Bindings, AfterHelm)
		if moduleHook.OrderByBinding[AfterHelm], ok = config.AfterHelm.(float64); !ok {
			return fmt.Errorf("unsuported value '%v' for binding '%s'", config.AfterHelm, AfterHelm)
		}
		mm.addModulesHooksOrderByName(moduleName, AfterHelm, moduleHook)
	}

	if config.AfterDeleteHelm != nil {
		moduleHook.Bindings = append(moduleHook.Bindings, AfterDeleteHelm)
		if moduleHook.OrderByBinding[AfterDeleteHelm], ok = config.AfterDeleteHelm.(float64); !ok {
			return fmt.Errorf("unsuported value '%v' for binding '%s'", config.AfterDeleteHelm, AfterDeleteHelm)
		}
		mm.addModulesHooksOrderByName(moduleName, AfterDeleteHelm, moduleHook)
	}

	if config.OnStartup != nil {
		moduleHook.Bindings = append(moduleHook.Bindings, OnStartup)
		if moduleHook.OrderByBinding[OnStartup], ok = config.OnStartup.(float64); !ok {
			return fmt.Errorf("unsuported value '%v' for binding '%s'", config.OnStartup, OnStartup)
		}
		mm.addModulesHooksOrderByName(moduleName, OnStartup, moduleHook)
	}

	if len(config.Schedule) != 0 {
		moduleHook.Bindings = append(moduleHook.Bindings, Schedule)
		mm.addModulesHooksOrderByName(moduleName, Schedule, moduleHook)
	}

	if len(config.OnKubernetesEvent) != 0 {
		moduleHook.Bindings = append(moduleHook.Bindings, KubeEvents)
		mm.addModulesHooksOrderByName(moduleName, KubeEvents, moduleHook)
	}

	return nil
}

func (mm *MainModuleManager) addModulesHooksOrderByName(moduleName string, bindingType BindingType, moduleHook *ModuleHook) {
	if mm.modulesHooksOrderByName[moduleName] == nil {
		mm.modulesHooksOrderByName[moduleName] = make(map[BindingType][]*ModuleHook)
	}
	mm.modulesHooksOrderByName[moduleName][bindingType] = append(mm.modulesHooksOrderByName[moduleName][bindingType], moduleHook)
}

func (mm *MainModuleManager) removeModuleHooks(moduleName string) {
	delete(mm.modulesHooksOrderByName, moduleName)
}


type globalValuesMergeResult struct {
	// Global values with the root "global" key.
	Values utils.Values
	// Global values under the root "global" key.
	GlobalValues map[string]interface{}
	// Original values patch argument.
	ValuesPatch utils.ValuesPatch
	// Whether values changed after applying patch.
	ValuesChanged bool
}

func (h *GlobalHook) handleGlobalValuesPatch(currentValues utils.Values, valuesPatch utils.ValuesPatch) (*globalValuesMergeResult, error) {
	acceptableKey := "global"

	if err := validateHookValuesPatch(valuesPatch, acceptableKey); err != nil {
		return nil, fmt.Errorf("merge global values failed: %s", err)
	}

	newValuesRaw, valuesChanged, err := utils.ApplyValuesPatch(currentValues, valuesPatch)
	if err != nil {
		return nil, fmt.Errorf("merge global values failed: %s", err)
	}

	result := &globalValuesMergeResult{
		Values:        utils.Values{acceptableKey: make(map[string]interface{})},
		ValuesChanged: valuesChanged,
		ValuesPatch:   valuesPatch,
	}

	if globalValuesRaw, hasKey := newValuesRaw[acceptableKey]; hasKey {
		globalValues, ok := globalValuesRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected map at key '%s', got:\n%s", acceptableKey, utils_data.YamlToString(globalValuesRaw))
		}

		result.Values[acceptableKey] = globalValues
		result.GlobalValues = globalValues
	}

	return result, nil
}

func (h *GlobalHook) run(bindingType BindingType, context []BindingContext) error {
	rlog.Infof("Running global hook '%s' binding '%s' ...", h.Name, bindingType)

	globalHookExecutor := NewHookExecutor(h, context)
	patches, err := globalHookExecutor.Run()
	if err != nil {
		return fmt.Errorf("global hook '%s' failed: %s", h.Name, err)
	}

	configValuesPatch, has := patches[utils.ConfigMapPatch]
	if has && configValuesPatch != nil {
		preparedConfigValues := utils.MergeValues(
			utils.Values{"global": map[string]interface{}{}},
			h.moduleManager.kubeGlobalConfigValues,
		)

		configValuesPatchResult, err := h.handleGlobalValuesPatch(preparedConfigValues, *configValuesPatch)
		if err != nil {
			return fmt.Errorf("global hook '%s': kube config global values update error: %s", h.Name, err)
		}

		if configValuesPatchResult.ValuesChanged {
			if err := h.moduleManager.kubeConfigManager.SetKubeGlobalValues(configValuesPatchResult.Values); err != nil {
				rlog.Debugf("Global hook '%s' kube config global values stay unchanged:\n%s", utils.ValuesToString(h.moduleManager.kubeGlobalConfigValues))
				return fmt.Errorf("global hook '%s': set kube config failed: %s", h.Name, err)
			}

			h.moduleManager.kubeGlobalConfigValues = configValuesPatchResult.Values
			rlog.Debugf("Global hook '%s': kube config global values updated:\n%s", h.Name, utils.ValuesToString(h.moduleManager.kubeGlobalConfigValues))
		}
	}

	valuesPatch, has := patches[utils.MemoryValuesPatch]
	if has && valuesPatch != nil {
		valuesPatchResult, err := h.handleGlobalValuesPatch(h.values(), *valuesPatch)
		if err != nil {
			return fmt.Errorf("global hook '%s': dynamic global values update error: %s", h.Name, err)
		}
		if valuesPatchResult.ValuesChanged {
			h.moduleManager.globalDynamicValuesPatches = utils.AppendValuesPatch(h.moduleManager.globalDynamicValuesPatches, valuesPatchResult.ValuesPatch)
			rlog.Debugf("Global hook '%s': global values updated:\n%s", h.Name, utils.ValuesToString(h.values()))
		}
	}

	return nil
}

// PrepareTmpFilesForHookRun creates temporary files for hook and returns environment variables with paths
func (h *GlobalHook) PrepareTmpFilesForHookRun(context []BindingContext) (tmpFiles map[string]string, err error) {
	tmpFiles = make(map[string]string, 0)

	tmpFiles["CONFIG_VALUES_PATH"], err = h.prepareConfigValuesJsonFile()
	if err != nil {
		return
	}

	tmpFiles["VALUES_PATH"], err = h.prepareValuesJsonFile()
	if err != nil {
		return
	}

	if len(context) > 0 {
		tmpFiles["BINDING_CONTEXT_PATH"], err = h.prepareBindingContextJsonFile(context)
		if err != nil {
			return
		}
	}

	tmpFiles["CONFIG_VALUES_JSON_PATCH_PATH"], err = h.prepareConfigValuesJsonPatchFile()
	if err != nil {
		return
	}

	tmpFiles["VALUES_JSON_PATCH_PATH"], err = h.prepareValuesJsonPatchFile()
	if err != nil {
		return
	}

	return
}


func (h *GlobalHook) configValues() utils.Values {
	return utils.MergeValues(
		utils.Values{"global": map[string]interface{}{}},
		h.moduleManager.kubeGlobalConfigValues,
	)
}

func (h *GlobalHook) values() utils.Values {
	var err error

	res := utils.MergeValues(
		utils.Values{"global": map[string]interface{}{}},
		h.moduleManager.globalCommonStaticValues,
		h.moduleManager.kubeGlobalConfigValues,
	)

	// Invariant: do not store patches that does not apply
	// Give user error for patches early, after patch receive
	for _, patch := range h.moduleManager.globalDynamicValuesPatches {
		res, _, err = utils.ApplyValuesPatch(res, patch)
		if err != nil {
			panic(err)
		}
	}

	return res
}

func (h *GlobalHook) prepareConfigValuesYamlFile() (string, error) {
	values := h.configValues()

	data := utils.MustDump(utils.DumpValuesYaml(values))
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("global-hook-%s-config-values.yaml", h.SafeName()))
	err := dumpData(path, data)
	if err != nil {
		return "", err
	}

	rlog.Debugf("Prepared global hook %s config values:\n%s", h.Name, utils.ValuesToString(values))

	return path, nil
}

func (h *GlobalHook) prepareConfigValuesJsonFile() (string, error) {
	values := h.configValues()

	data := utils.MustDump(utils.DumpValuesJson(values))
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("global-hook-%s-config-values.json", h.SafeName()))
	err := dumpData(path, data)
	if err != nil {
		return "", err
	}

	rlog.Debugf("Prepared global hook %s config values:\n%s", h.Name, utils.ValuesToString(values))

	return path, nil
}

func (h *GlobalHook) prepareValuesYamlFile() (string, error) {
	values := h.values()

	data := utils.MustDump(utils.DumpValuesYaml(values))
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("global-hook-%s-values.yaml", h.SafeName()))
	err := dumpData(path, data)
	if err != nil {
		return "", err
	}

	rlog.Debugf("Prepared global hook %s values:\n%s", h.Name, utils.ValuesToString(values))

	return path, nil
}

func (h *GlobalHook) prepareValuesJsonFile() (string, error) {
	values := h.values()

	data := utils.MustDump(utils.DumpValuesJson(values))
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("global-hook-%s-values.json", h.SafeName()))
	err := dumpData(path, data)
	if err != nil {
		return "", err
	}

	rlog.Debugf("Prepared global hook %s values:\n%s", h.Name, utils.ValuesToString(values))

	return path, nil
}

func (h *GlobalHook) prepareBindingContextJsonFile(context []BindingContext) (string, error) {
	data, _ := json.Marshal(context)
	//data := utils.MustDump(utils.DumpValuesJson(context))
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("global-hook-%s-binding-context.json", h.SafeName()))
	err := dumpData(path, data)
	if err != nil {
		return "", err
	}

	rlog.Debugf("Prepared global hook %s binding context:\n%s", h.Name, utils_data.YamlToString(context))

	return path, nil
}

type moduleValuesMergeResult struct {
	// global values with root ModuleValuesKey key
	Values utils.Values
	// global values under root ModuleValuesKey key
	ModuleValues    map[string]interface{}
	ModuleValuesKey string
	ValuesPatch     utils.ValuesPatch
	ValuesChanged   bool
}

func (h *CommonHook) SafeName() string {
	return sanitize.BaseName(h.Name)
}

func (h *CommonHook) GetName() string {
	return h.Name
}

func (h *CommonHook) GetPath() string {
	return h.Path
}

func (h *ModuleHook) handleModuleValuesPatch(currentValues utils.Values, valuesPatch utils.ValuesPatch) (*moduleValuesMergeResult, error) {
	moduleValuesKey := utils.ModuleNameToValuesKey(h.Module.Name)

	if err := validateHookValuesPatch(valuesPatch, moduleValuesKey); err != nil {
		return nil, fmt.Errorf("merge module '%s' values failed: %s", h.Module.Name, err)
	}

	newValuesRaw, valuesChanged, err := utils.ApplyValuesPatch(currentValues, valuesPatch)
	if err != nil {
		return nil, fmt.Errorf("merge module '%s' values failed: %s", h.Module.Name, err)
	}

	result := &moduleValuesMergeResult{
		ModuleValuesKey: moduleValuesKey,
		Values:          utils.Values{moduleValuesKey: make(map[string]interface{})},
		ValuesChanged:   valuesChanged,
		ValuesPatch:     valuesPatch,
	}

	if moduleValuesRaw, hasKey := newValuesRaw[result.ModuleValuesKey]; hasKey {
		moduleValues, ok := moduleValuesRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected map at key '%s', got:\n%s", result.ModuleValuesKey, utils_data.YamlToString(moduleValuesRaw))
		}
		result.Values[result.ModuleValuesKey] = moduleValues
		result.ModuleValues = moduleValues
	}

	return result, nil
}

func validateHookValuesPatch(valuesPatch utils.ValuesPatch, acceptableKey string) error {
	for _, op := range valuesPatch.Operations {
		if op.Op == "replace" {
			return fmt.Errorf("unsupported patch operation '%s': '%s'", op.Op, op.ToString())
		}

		pathParts := strings.Split(op.Path, "/")
		if len(pathParts) > 1 {
			affectedKey := pathParts[1]
			if affectedKey != acceptableKey {
				return fmt.Errorf("unacceptable patch operation path '%s' (only '%s' accepted): '%s'", affectedKey, acceptableKey, op.ToString())
			}
		}
	}

	return nil
}

func (h *ModuleHook) run(bindingType BindingType, context []BindingContext) error {
	moduleName := h.Module.Name
	rlog.Infof("Running module hook '%s' binding '%s' ...", h.Name, bindingType)

	moduleHookExecutor := NewHookExecutor(h, context)
	patches, err := moduleHookExecutor.Run()
	if err != nil {
		return fmt.Errorf("module hook '%s' failed: %s", h.Name, err)
	}

	configValuesPatch, has := patches[utils.ConfigMapPatch]
	if has && configValuesPatch != nil{
		preparedConfigValues := utils.MergeValues(
			utils.Values{utils.ModuleNameToValuesKey(moduleName): map[string]interface{}{}},
			h.moduleManager.kubeModulesConfigValues[moduleName],
		)

		configValuesPatchResult, err := h.handleModuleValuesPatch(preparedConfigValues, *configValuesPatch)
		if err != nil {
			return fmt.Errorf("module hook '%s': kube module config values update error: %s", h.Name, err)
		}
		if configValuesPatchResult.ValuesChanged {
			err := h.moduleManager.kubeConfigManager.SetKubeModuleValues(moduleName, configValuesPatchResult.Values)
			if err != nil {
				rlog.Debugf("Module hook '%s' kube module config values stay unchanged:\n%s", utils.ValuesToString(h.moduleManager.kubeModulesConfigValues[moduleName]))
				return fmt.Errorf("module hook '%s': set kube module config failed: %s", h.Name, err)
			}

			h.moduleManager.kubeModulesConfigValues[moduleName] = configValuesPatchResult.Values
			rlog.Debugf("Module hook '%s': kube module '%s' config values updated:\n%s", h.Name, moduleName, utils.ValuesToString(h.moduleManager.kubeModulesConfigValues[moduleName]))
		}
	}

	valuesPatch, has := patches[utils.MemoryValuesPatch]
	if has && valuesPatch != nil {
		valuesPatchResult, err := h.handleModuleValuesPatch(h.values(), *valuesPatch)
		if err != nil {
			return fmt.Errorf("module hook '%s': dynamic module values update error: %s", h.Name, err)
		}
		if valuesPatchResult.ValuesChanged {
			h.moduleManager.modulesDynamicValuesPatches[moduleName] = utils.AppendValuesPatch(h.moduleManager.modulesDynamicValuesPatches[moduleName], valuesPatchResult.ValuesPatch)
			rlog.Debugf("Module hook '%s': dynamic module '%s' values updated:\n%s", h.Name, moduleName, utils.ValuesToString(h.values()))
		}
	}

	return nil
}

// PrepareTmpFilesForHookRun creates temporary files for hook and returns environment variables with paths
func (h *ModuleHook) PrepareTmpFilesForHookRun(context []BindingContext) (tmpFiles map[string]string, err error) {
	tmpFiles = make(map[string]string, 0)

	tmpFiles["CONFIG_VALUES_PATH"], err = h.prepareConfigValuesJsonFile()
	if err != nil {
		return
	}

	tmpFiles["VALUES_PATH"], err = h.prepareValuesJsonFile()
	if err != nil {
		return
	}

	if len(context) > 0 {
		tmpFiles["BINDING_CONTEXT_PATH"], err = h.prepareBindingContextJsonFile(context)
		if err != nil {
			return
		}
	}

	tmpFiles["CONFIG_VALUES_JSON_PATCH_PATH"], err= h.prepareConfigValuesJsonPatchFile()
	if err != nil {
		return
	}

	tmpFiles["VALUES_JSON_PATCH_PATH"], err = h.prepareValuesJsonPatchFile()
	if err != nil {
		return
	}

	return
}


func (h *ModuleHook) configValues() utils.Values {
	return h.Module.configValues()
}

func (h *ModuleHook) values() utils.Values {
	return h.Module.values()
}

func (h *ModuleHook) prepareValuesJsonFile() (string, error) {
	return h.Module.prepareValuesJsonFile()
}

func (h *ModuleHook) prepareValuesYamlFile() (string, error) {
	return h.Module.prepareValuesYamlFile()
}

func (h *ModuleHook) prepareConfigValuesJsonFile() (string, error) {
	return h.Module.prepareConfigValuesJsonFile()
}

func (h *ModuleHook) prepareConfigValuesYamlFile() (string, error) {
	return h.Module.prepareConfigValuesYamlFile()
}

func (h *ModuleHook) prepareBindingContextJsonFile(context []BindingContext) (string, error) {
	data, _ := json.Marshal(context)
	//data := utils.MustDump(utils.DumpValuesJson(context))
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("%s.module-hook-%s-binding-context.json", h.Module.SafeName(), h.SafeName()))
	err := dumpData(path, data)
	if err != nil {
		return "", err
	}

	rlog.Debugf("Prepared module %s hook %s binding context:\n%s", h.Module.SafeName(), h.Name, utils_data.YamlToString(context))

	return path, nil
}

func prepareHookConfig(hookConfig *HookConfig) {
	for i := range hookConfig.OnKubernetesEvent {
		config := &hookConfig.OnKubernetesEvent[i]

		if config.EventTypes == nil {
			config.EventTypes = []kube_events_manager.OnKubernetesEventType{
				kube_events_manager.KubernetesEventOnAdd,
				kube_events_manager.KubernetesEventOnUpdate,
				kube_events_manager.KubernetesEventOnDelete,
			}
		}

		if config.NamespaceSelector == nil {
			config.NamespaceSelector = &kube_events_manager.KubeNamespaceSelector{Any: true}
		}
	}
}

func (mm *MainModuleManager) initGlobalHooks() error {
	rlog.Debug("INIT: global hooks")

	mm.globalHooksOrder = make(map[BindingType][]*GlobalHook)
	mm.globalHooksByName = make(map[string]*GlobalHook)

	hooksDir := mm.GlobalHooksDir

	err := mm.initHooks(hooksDir, func(hookPath string, output []byte) error {
		hookName, err := filepath.Rel(mm.GlobalHooksDir, hookPath)
		if err != nil {
			return err
		}

		rlog.Infof("INIT: global hook '%s'", hookName)

		hookConfig := &GlobalHookConfig{}
		if err := json.Unmarshal(output, hookConfig); err != nil {
			return fmt.Errorf("INIT: cannot unmarshal config from global hook %s: %s\n%s", hookName, err.Error(), output)
		}

		prepareHookConfig(&hookConfig.HookConfig)

		if err := mm.registerGlobalHook(hookName, hookPath, hookConfig); err != nil {
			return fmt.Errorf("INIT: cannot add global hook '%s': %s", hookName, err.Error())
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

func (mm *MainModuleManager) initModuleHooks(module *Module) error {
	if _, ok := mm.modulesHooksOrderByName[module.Name]; ok {
		rlog.Debugf("INIT: module '%s' hooks: already initialized", module.Name)
		return nil
	}

	rlog.Infof("INIT: module '%s' hooks ...", module.Name)

	hooksDir := filepath.Join(module.Path, "hooks")

	err := mm.initHooks(hooksDir, func(hookPath string, output []byte) error {
		hookName, err := filepath.Rel(filepath.Dir(module.Path), hookPath)
		if err != nil {
			return err
		}

		rlog.Infof("INIT:   hook '%s' ...", hookName)

		hookConfig := &ModuleHookConfig{}
		if err := json.Unmarshal(output, hookConfig); err != nil {
			return fmt.Errorf("unmarshaling module '%s' hook '%s' json failed: %s", module.SafeName(), hookName, err.Error())
		}

		prepareHookConfig(&hookConfig.HookConfig)

		if err := mm.registerModuleHook(module.Name, hookName, hookPath, hookConfig); err != nil {
			return fmt.Errorf("adding module '%s' hook '%s' failed: %s", module.SafeName(), hookName, err.Error())
		}

		return nil
	})

	if err != nil {
		// cleanup hook indexes on error
		mm.removeModuleHooks(module.Name)
		return err
	}

	return nil
}

func (mm *MainModuleManager) initHooks(hooksDir string, addHookFn func(hookPath string, output []byte) error) error {
	if _, err := os.Stat(hooksDir); os.IsNotExist(err) {
		return nil
	}

	// retrieve a list of executable files in hooksDir sorted by filename
	hooksRelativePaths, _, err := utils.FindExecutableFilesInPath(hooksDir)
	if err != nil {
		return err
	}

	for _, hookPath := range hooksRelativePaths {
		cmd := makeCommand("", hookPath, []string{}, []string{"--config"})
		output, err := execCommandOutput(cmd)
		if err != nil {
			return fmt.Errorf("cannot get config for hook '%s': %s", hookPath, err)
		}

		if err := addHookFn(hookPath, output); err != nil {
			return err
		}
	}


	return nil
}

func (h *GlobalHook) prepareConfigValuesJsonPatchFile() (string, error) {
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("%s.global-hook-config-values.json-patch", h.SafeName()))
	if err := createHookResultValuesFile(path); err != nil {
		return "", err
	}
	return path, nil
}

func (h *GlobalHook) prepareValuesJsonPatchFile() (string, error) {
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("%s.global-hook-values.json-patch", h.SafeName()))
	if err := createHookResultValuesFile(path); err != nil {
		return "", err
	}
	return path, nil
}

func (h *ModuleHook) prepareConfigValuesJsonPatchFile() (string, error) {
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("%s.global-hook-config-values.json-patch", h.SafeName()))
	if err := createHookResultValuesFile(path); err != nil {
		return "", err
	}
	return path, nil
}

func (h *ModuleHook) prepareValuesJsonPatchFile() (string, error) {
	path := filepath.Join(h.moduleManager.TempDir, fmt.Sprintf("%s.global-hook-values.json-patch", h.SafeName()))
	if err := createHookResultValuesFile(path); err != nil {
		return "", err
	}
	return path, nil
}


func createHookResultValuesFile(filePath string) error {
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return nil
	}

	_ = file.Close()
	return nil
}

func makeCommand(dir string, entrypoint string, envs []string, args []string) *exec.Cmd {
	envs = append(os.Environ(), envs...)
	return executor.MakeCommand(dir, entrypoint, args, envs)
}

func execCommandOutput(cmd *exec.Cmd) ([]byte, error) {
	rlog.Debugf("Executing hook in %s: '%s'", cmd.Dir, strings.Join(cmd.Args, " "))
	cmd.Stdout = nil

	output, err := executor.Output(cmd)
	if err != nil {
		rlog.Errorf("Hook '%s' output:\n%s", strings.Join(cmd.Args, " "), string(output))
		return output, err
	}

	rlog.Debugf("Hook '%s' output:\n%s", strings.Join(cmd.Args, " "), string(output))

	return output, nil
}


type HookExecutor struct {
	Hook Hook
	Context []BindingContext
	ConfigValuesPath string
	ValuesPath string
	ContextPath string
	ConfigValuesPatchPath string
	ValuesPatchPath string
}

func NewHookExecutor(h Hook, context []BindingContext) *HookExecutor {
	return &HookExecutor{
		Hook: h,
		Context: context,
	}
}

func (e *HookExecutor) Run() (patches map[utils.ValuesPatchType]*utils.ValuesPatch, err error) {
	patches = make(map[utils.ValuesPatchType]*utils.ValuesPatch)

	tmpFiles, err := e.Hook.PrepareTmpFilesForHookRun(e.Context)
	if err != nil {
		return nil, err
	}
	e.ConfigValuesPatchPath = tmpFiles["CONFIG_VALUES_JSON_PATCH_PATH"]
	e.ValuesPatchPath = tmpFiles["VALUES_JSON_PATCH_PATH"]

	envs := []string{}
	envs = append(envs, os.Environ()...)
	for envName, filePath := range tmpFiles {
		envs = append(envs, fmt.Sprintf("%s=%s", envName, filePath))
	}
	envs = append(envs, helm.Client.CommandEnv()...)

	cmd := executor.MakeCommand("", e.Hook.GetPath(), []string{}, envs)

	err = executor.Run(cmd, true)
	if err != nil {
		return nil, fmt.Errorf("%s FAILED: %s", e.Hook.GetName(), err)
	}

	patches[utils.ConfigMapPatch], err = utils.ValuesPatchFromFile(e.ConfigValuesPatchPath)
	if err != nil {
		return nil, fmt.Errorf("got bad config values json patch from hook %s: %s", e.Hook.GetName(), err)
	}

	patches[utils.MemoryValuesPatch], err = utils.ValuesPatchFromFile(e.ValuesPatchPath)
	if err != nil {
		return nil, fmt.Errorf("got bad values json patch from hook %s: %s", e.Hook.GetName(), err)
	}

	return patches, nil
}
