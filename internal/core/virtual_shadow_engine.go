package core

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/expr-lang/expr"
)

// pointRefPrefixPattern matches channel.device. prefix; point segment may contain OPC UA node id chars (= ; .).
var pointRefPrefixPattern = regexp.MustCompile(`[a-zA-Z0-9][a-zA-Z0-9_-]*\.[a-zA-Z0-9][a-zA-Z0-9_-]*\.`)

// parsePointRef splits channel.device.point where point may contain dots (e.g. OPC UA ns=2;s=Some.Node).
func parsePointRef(ref string) (channelID, deviceID, pointID string, ok bool) {
	ref = strings.TrimSpace(ref)
	first := strings.IndexByte(ref, '.')
	if first <= 0 {
		return "", "", "", false
	}
	rest := ref[first+1:]
	second := strings.IndexByte(rest, '.')
	if second <= 0 {
		return "", "", "", false
	}
	second = first + 1 + second
	channelID = ref[:first]
	deviceID = ref[first+1 : second]
	pointID = ref[second+1:]
	if channelID == "" || deviceID == "" || pointID == "" {
		return "", "", "", false
	}
	return channelID, deviceID, pointID, true
}

func isPointRefBoundary(b byte) bool {
	switch b {
	case ' ', '+', '-', '*', '/', '(', ')':
		return true
	default:
		return false
	}
}

func extendPointRefEnd(formula string, startAfterPrefix int) int {
	i := startAfterPrefix
	for i < len(formula) {
		switch formula[i] {
		case ' ', '+', '-', '*', '/', '(', ')':
			return i
		default:
			i++
		}
	}
	return i
}

func extractPointRefAt(formula string, prefixLoc []int) (string, bool) {
	start := prefixLoc[0]
	if start > 0 && !isPointRefBoundary(formula[start-1]) {
		return "", false
	}
	end := extendPointRefEnd(formula, prefixLoc[1])
	ref := formula[start:end]
	if _, _, _, ok := parsePointRef(ref); !ok {
		return "", false
	}
	return ref, true
}

func isMapModeRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	loc := pointRefPrefixPattern.FindStringIndex(ref)
	if loc == nil || loc[0] != 0 {
		return false
	}
	end := extendPointRefEnd(ref, loc[1])
	return end == len(ref)
}

type VirtualShadowEngine struct {
	mu sync.RWMutex

	virtualDevices  map[string]*model.VirtualDevice
	dependencyGraph map[string][]string
	shadowCore      *ShadowCore

	formulaCache map[string]interface{}
}

func NewVirtualShadowEngine(sc *ShadowCore) *VirtualShadowEngine {
	vse := &VirtualShadowEngine{
		virtualDevices:  make(map[string]*model.VirtualDevice),
		dependencyGraph: make(map[string][]string),
		shadowCore:      sc,
		formulaCache:    make(map[string]interface{}),
	}

	sc.Subscribe(vse.handleShadowUpdate)

	return vse
}

func (vse *VirtualShadowEngine) CreateVirtualDevice(deviceID, channelID string, formulaPoints map[string]string) error {
	vse.mu.Lock()
	defer vse.mu.Unlock()

	if _, exists := vse.virtualDevices[deviceID]; exists {
		return fmt.Errorf("virtual device already exists: %s", deviceID)
	}

	dependencies := vse.extractDependencies(formulaPoints)
	if channelID == "" {
		channelID = inferChannelFromDependencies(dependencies)
	}

	device := &model.VirtualDevice{
		VirtualDeviceID: deviceID,
		ChannelID:       channelID,
		Version:         0,
		UpdatedAt:       time.Now(),
		FormulaPoints:   formulaPoints,
		Dependencies:    dependencies,
		Points:          make(map[string]model.ShadowPoint),
	}

	vse.virtualDevices[deviceID] = device

	for _, dep := range dependencies {
		vse.dependencyGraph[dep] = append(vse.dependencyGraph[dep], deviceID)
	}

	log.Printf("[VirtualShadowEngine] Created virtual device: %s with %d dependencies", deviceID, len(dependencies))

	go vse.recomputeVirtualDevice(deviceID)

	return nil
}

// ReplaceVirtualDevice 全量替换虚拟设备公式并触发重算。
func (vse *VirtualShadowEngine) ReplaceVirtualDevice(deviceID, channelID string, formulaPoints map[string]string) error {
	_ = vse.DeleteVirtualDevice(deviceID)
	return vse.CreateVirtualDevice(deviceID, channelID, formulaPoints)
}

func inferChannelFromDependencies(deps []string) string {
	for _, dep := range deps {
		if ch, _, _, ok := parsePointRef(dep); ok {
			return ch
		}
	}
	return ""
}

func (vse *VirtualShadowEngine) extractDependencies(formulaPoints map[string]string) []string {
	depSet := make(map[string]bool)

	for _, formula := range formulaPoints {
		refs := vse.parseFormulaReferences(formula)
		for _, ref := range refs {
			depSet[ref] = true
		}
	}

	deps := make([]string, 0, len(depSet))
	for dep := range depSet {
		deps = append(deps, dep)
	}
	return deps
}

func (vse *VirtualShadowEngine) parseFormulaReferences(formula string) []string {
	seen := make(map[string]struct{})
	var refs []string
	for _, loc := range pointRefPrefixPattern.FindAllStringIndex(formula, -1) {
		ref, ok := extractPointRefAt(formula, loc)
		if !ok {
			continue
		}
		if _, dup := seen[ref]; dup {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func isNumber(s string) bool {
	return strings.Contains(s, ".") && len(strings.Split(s, ".")) == 2
}

func (vse *VirtualShadowEngine) handleShadowUpdate(shadowDeviceID string, points map[string]model.ShadowPoint) {
	if IsVirtualShadowID(shadowDeviceID) {
		return
	}

	device, err := vse.shadowCore.GetShadowDevice(shadowDeviceID)
	if err != nil {
		return
	}

	affected := make(map[string]struct{})
	vse.mu.RLock()
	for pointID := range points {
		depKey := fmt.Sprintf("%s.%s.%s", device.ChannelID, device.PhysicalDeviceID, pointID)
		for _, vdID := range vse.dependencyGraph[depKey] {
			affected[vdID] = struct{}{}
		}
	}
	vse.mu.RUnlock()

	for vdID := range affected {
		go vse.recomputeVirtualDevice(vdID)
	}
}

func (vse *VirtualShadowEngine) recomputeVirtualDevice(deviceID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[VirtualShadowEngine] panic in recomputeVirtualDevice %s (recovered): %v", deviceID, r)
		}
	}()

	vse.mu.Lock()
	defer vse.mu.Unlock()

	device, exists := vse.virtualDevices[deviceID]
	if !exists {
		return
	}

	env := vse.buildEvaluationEnv(device.Dependencies)

	updatedPoints := make(map[string]model.ShadowPoint)
	now := time.Now()

	for pointID, formula := range device.FormulaPoints {
		trimmed := strings.TrimSpace(formula)
		sourcesGood := vse.formulaSourcesGood(trimmed, device.Dependencies)
		quality := "good"
		if !sourcesGood {
			quality = "bad"
		}

		var result interface{}
		var err error

		// 映射模式：公式即 channel.device.point，直接透传，避免 expr 误解析 '-' '.'
		if isMapModeRef(trimmed) {
			key := depToEnvKey(trimmed)
			val, ok := env[key]
			if !ok {
				if prev, has := device.Points[pointID]; has {
					result = prev.Value
				}
			} else {
				result = val
			}
		} else {
			if !sourcesGood {
				if prev, has := device.Points[pointID]; has {
					result = prev.Value
				}
			} else {
				rewritten := rewriteFormulaDeps(formula, device.Dependencies)
				result, err = vse.evaluateFormula(rewritten, env)
				if err != nil {
					log.Printf("[VirtualShadowEngine] Formula evaluation failed for %s.%s: %v", deviceID, pointID, err)
					if prev, has := device.Points[pointID]; has {
						result = prev.Value
					}
					quality = "bad"
				}
			}
		}

		device.Version++
		pt := model.ShadowPoint{
			Value:       result,
			Timestamp:   now,
			CollectedAt: now,
			UpdatedAt:   now,
			Version:     device.Version,
			Quality:     quality,
		}
		device.Points[pointID] = pt
		updatedPoints[pointID] = pt
	}

	if len(updatedPoints) > 0 {
		device.UpdatedAt = now
		//log.Printf("[VirtualShadowEngine] Recomputed virtual device: %s, version: %d", deviceID, device.Version)
		vse.shadowCore.WriteVirtualShadowDevice(device.ChannelID, deviceID, updatedPoints)
	}
}

func isShadowQualityGood(q string) bool {
	return strings.EqualFold(q, "good")
}

func depsForFormula(formula string, dependencies []string) []string {
	trimmed := strings.TrimSpace(formula)
	if isMapModeRef(trimmed) {
		return []string{trimmed}
	}
	var out []string
	for _, dep := range dependencies {
		if strings.Contains(formula, dep) {
			out = append(out, dep)
		}
	}
	return out
}

func (vse *VirtualShadowEngine) resolveSourcePoint(dep string) (*model.ShadowPoint, bool) {
	deviceID, pointID := parseDepRef(dep)
	if deviceID == "" || pointID == "" {
		return nil, false
	}
	shadowDeviceID := fmt.Sprintf("shadow-%s", deviceID)
	shadowDevice, err := vse.shadowCore.GetShadowDevice(shadowDeviceID)
	if err != nil {
		return nil, false
	}
	point, exists := shadowDevice.Points[pointID]
	if !exists {
		return nil, false
	}
	return &point, true
}

func (vse *VirtualShadowEngine) formulaSourcesGood(formula string, dependencies []string) bool {
	for _, dep := range depsForFormula(formula, dependencies) {
		pt, ok := vse.resolveSourcePoint(dep)
		if !ok || !isShadowQualityGood(pt.Quality) {
			return false
		}
	}
	return true
}

func (vse *VirtualShadowEngine) buildEvaluationEnv(dependencies []string) map[string]interface{} {
	env := make(map[string]interface{})

	for _, dep := range dependencies {
		pt, ok := vse.resolveSourcePoint(dep)
		if !ok {
			continue
		}
		env[depToEnvKey(dep)] = pt.Value
	}

	return env
}

func parseDepRef(dep string) (deviceID, pointID string) {
	if _, deviceID, pointID, ok := parsePointRef(dep); ok {
		return deviceID, pointID
	}
	parts := strings.Split(dep, ".")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

func depToEnvKey(dep string) string {
	return strings.NewReplacer(".", "_", "-", "_").Replace(dep)
}

func rewriteFormulaDeps(formula string, deps []string) string {
	sorted := append([]string(nil), deps...)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i]) > len(sorted[j])
	})
	result := formula
	for _, dep := range sorted {
		result = strings.ReplaceAll(result, dep, depToEnvKey(dep))
	}
	return result
}

func (vse *VirtualShadowEngine) evaluateFormula(formula string, env map[string]interface{}) (interface{}, error) {
	program, err := expr.Compile(formula, expr.Env(env))
	if err != nil {
		return nil, fmt.Errorf("compile error: %w", err)
	}

	result, err := expr.Run(program, env)
	if err != nil {
		return nil, fmt.Errorf("run error: %w", err)
	}

	return result, nil
}

// RecomputeDevice 同步重算虚拟设备点位（供 API 刷新当前值）。
func (vse *VirtualShadowEngine) RecomputeDevice(deviceID string) {
	vse.recomputeVirtualDevice(deviceID)
}

func (vse *VirtualShadowEngine) GetVirtualDevice(deviceID string) (*model.VirtualDevice, error) {
	vse.mu.RLock()
	defer vse.mu.RUnlock()

	device, exists := vse.virtualDevices[deviceID]
	if !exists {
		return nil, fmt.Errorf("virtual device not found: %s", deviceID)
	}

	vd := *device
	vd.Points = cloneShadowPoints(device.Points)
	vd.FormulaPoints = make(map[string]string, len(device.FormulaPoints))
	for k, v := range device.FormulaPoints {
		vd.FormulaPoints[k] = v
	}
	vd.Dependencies = make([]string, len(device.Dependencies))
	copy(vd.Dependencies, device.Dependencies)
	return &vd, nil
}

func (vse *VirtualShadowEngine) GetAllVirtualDevices() []*model.VirtualDevice {
	vse.mu.RLock()
	defer vse.mu.RUnlock()

	result := make([]*model.VirtualDevice, 0, len(vse.virtualDevices))
	for _, device := range vse.virtualDevices {
		vd := *device
		vd.Points = cloneShadowPoints(device.Points)
		vd.FormulaPoints = make(map[string]string, len(device.FormulaPoints))
		for k, v := range device.FormulaPoints {
			vd.FormulaPoints[k] = v
		}
		vd.Dependencies = make([]string, len(device.Dependencies))
		copy(vd.Dependencies, device.Dependencies)
		result = append(result, &vd)
	}
	return result
}

func (vse *VirtualShadowEngine) DeleteVirtualDevice(deviceID string) error {
	vse.mu.Lock()
	defer vse.mu.Unlock()

	device, exists := vse.virtualDevices[deviceID]
	if !exists {
		return fmt.Errorf("virtual device not found: %s", deviceID)
	}

	for _, dep := range device.Dependencies {
		affected := vse.dependencyGraph[dep]
		newAffected := make([]string, 0)
		for _, vdID := range affected {
			if vdID != deviceID {
				newAffected = append(newAffected, vdID)
			}
		}
		vse.dependencyGraph[dep] = newAffected
	}

	delete(vse.virtualDevices, deviceID)

	log.Printf("[VirtualShadowEngine] Deleted virtual device: %s", deviceID)

	return nil
}

func (vse *VirtualShadowEngine) UpdateFormula(deviceID, pointID, newFormula string) error {
	vse.mu.Lock()
	defer vse.mu.Unlock()

	device, exists := vse.virtualDevices[deviceID]
	if !exists {
		return fmt.Errorf("virtual device not found: %s", deviceID)
	}

	oldFormula := device.FormulaPoints[pointID]
	oldDeps := vse.parseFormulaReferences(oldFormula)

	for _, dep := range oldDeps {
		affected := vse.dependencyGraph[dep]
		newAffected := make([]string, 0)
		for _, vdID := range affected {
			if vdID != deviceID {
				newAffected = append(newAffected, vdID)
			}
		}
		if len(newAffected) == 0 {
			delete(vse.dependencyGraph, dep)
		} else {
			vse.dependencyGraph[dep] = newAffected
		}
	}

	device.FormulaPoints[pointID] = newFormula

	newDeps := vse.parseFormulaReferences(newFormula)
	for _, dep := range newDeps {
		vse.dependencyGraph[dep] = append(vse.dependencyGraph[dep], deviceID)
	}

	device.Dependencies = vse.extractDependencies(device.FormulaPoints)
	device.Version++
	device.UpdatedAt = time.Now()

	go vse.recomputeVirtualDevice(deviceID)

	log.Printf("[VirtualShadowEngine] Updated formula for %s.%s", deviceID, pointID)

	return nil
}

func (vse *VirtualShadowEngine) GetDependencyGraph() map[string][]string {
	vse.mu.RLock()
	defer vse.mu.RUnlock()

	result := make(map[string][]string)
	for k, v := range vse.dependencyGraph {
		result[k] = append([]string{}, v...)
	}
	return result
}

func (vse *VirtualShadowEngine) GetMetrics() map[string]interface{} {
	vse.mu.RLock()
	defer vse.mu.RUnlock()

	totalFormulas := 0
	for _, device := range vse.virtualDevices {
		totalFormulas += len(device.FormulaPoints)
	}

	return map[string]interface{}{
		"virtual_device_count": len(vse.virtualDevices),
		"total_formulas":       totalFormulas,
		"dependency_count":     len(vse.dependencyGraph),
	}
}
