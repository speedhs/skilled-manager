package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DoctorCheck is one diagnostic row.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// DoctorReport is the result of Doctor.
type DoctorReport struct {
	SchemaVersion int           `json:"schema_version"`
	Checks        []DoctorCheck `json:"checks"`
	Warnings      []string      `json:"warnings,omitempty"`
}

// Doctor checks the canonical directory, config, targets, links, and lock.
func (m *Manager) Doctor(ctx context.Context) (*DoctorReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	release, err := m.acquireLock()
	if err != nil {
		if errors.Is(err, ErrLocked) {
			return &DoctorReport{
				SchemaVersion: JSONSchemaVersion,
				Checks: []DoctorCheck{{
					Name: "lock", Status: "fail",
					Detail: err.Error(),
					Fix:    "Close the other skills process and run doctor again.",
				}},
			}, nil
		}
		return nil, err
	}
	defer release()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.reloadLocked(); err != nil {
		return nil, err
	}
	report := &DoctorReport{SchemaVersion: JSONSchemaVersion, Warnings: append([]string{}, m.warnings...)}
	report.Checks = append(report.Checks, m.checkWritable("canonical", m.CanonicalDir, "Create the directory or pick another path with --canonical."))
	report.Checks = append(report.Checks, m.checkWritable("config", m.ConfigDir, "Create the directory or pick another path with --config."))
	report.Checks = append(report.Checks, m.checkSymlink())
	if err := validateTargetSet(m.CanonicalDir, m.ConfigDir, m.targets); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			Name: "targets", Status: "fail", Detail: err.Error(),
			Fix: "Edit targets so every path is absolute and does not overlap another target, the canonical store, or the config directory.",
		})
	}
	for _, target := range m.targets {
		report.Checks = append(report.Checks, m.checkTarget(target))
	}
	report.Checks = append(report.Checks, m.checkState()...)
	return report, nil
}

func (m *Manager) checkWritable(name, dir, fix string) DoctorCheck {
	if m.dryRun {
		return DoctorCheck{Name: name, Status: "ok", Detail: "dry-run: " + dir}
	}
	probe := filepath.Join(dir, ".doctor-write-"+randHex(2))
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return DoctorCheck{Name: name, Status: "fail", Detail: err.Error(), Fix: fix}
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return DoctorCheck{Name: name, Status: "ok", Detail: dir}
}

func (m *Manager) checkSymlink() DoctorCheck {
	if m.dryRun {
		return DoctorCheck{Name: "symlink", Status: "ok", Detail: "dry-run: symlink creation was not probed"}
	}
	dir, err := os.MkdirTemp(m.ConfigDir, "symlink-check-")
	if err != nil {
		return DoctorCheck{Name: "symlink", Status: "fail", Detail: err.Error()}
	}
	defer os.RemoveAll(dir)
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		return DoctorCheck{Name: "symlink", Status: "fail", Detail: err.Error()}
	}
	if err := os.Symlink(src, dst); err != nil {
		return DoctorCheck{
			Name: "symlink", Status: "warn",
			Detail: "this process cannot create symlinks: " + err.Error(),
			Fix:    "On Windows, enable Developer Mode or run elevated. Skills Manager will use a junction or copy when a target is enabled.",
		}
	}
	return DoctorCheck{Name: "symlink", Status: "ok", Detail: "symlinks can be created"}
}

func (m *Manager) checkTarget(target Target) DoctorCheck {
	name := "target:" + target.ID
	expanded, err := ExpandPath(target.Path)
	if err != nil {
		return DoctorCheck{Name: name, Status: "fail", Detail: err.Error(), Fix: "Edit the target path in settings or targets.json."}
	}
	if !filepath.IsAbs(expanded) {
		return DoctorCheck{Name: name, Status: "fail", Detail: expanded + " is not an absolute path", Fix: "Set an absolute path in targets.json."}
	}
	info, err := os.Lstat(expanded)
	if err != nil {
		if os.IsNotExist(err) {
			return DoctorCheck{Name: name, Status: "warn", Detail: expanded + " does not exist yet", Fix: "It will be created the first time a skill is enabled for this target."}
		}
		return DoctorCheck{Name: name, Status: "fail", Detail: err.Error()}
	}
	if !info.IsDir() {
		return DoctorCheck{Name: name, Status: "fail", Detail: expanded + " is not a directory"}
	}
	detail := fmt.Sprintf("%s (%s, %s)", expanded, target.Mode, target.Format)
	for _, cat := range DefaultTargets() {
		if cat.ID == target.ID && cat.Path != target.Path {
			return DoctorCheck{
				Name: name, Status: "warn", Detail: detail + "; catalog path is " + cat.Path,
				Fix: "Update the path if you want new installs to follow the current upstream directory. Existing targets.json is not rewritten automatically.",
			}
		}
	}
	return DoctorCheck{Name: name, Status: "ok", Detail: detail}
}

func (m *Manager) checkState() []DoctorCheck {
	var checks []DoctorCheck
	skills, err := ListSkills(m.CanonicalDir)
	if err != nil {
		return []DoctorCheck{{Name: "skills", Status: "fail", Detail: err.Error()}}
	}
	known := map[string]struct{}{}
	for _, skill := range skills {
		known[skill.Name] = struct{}{}
		for _, target := range m.targets {
			st, err := m.statusLocked(skill.Name, target.ID)
			if err != nil {
				checks = append(checks, DoctorCheck{Name: skill.Name + "/" + target.ID, Status: "fail", Detail: err.Error()})
				continue
			}
			switch st.Code {
			case StatusBroken:
				checks = append(checks, DoctorCheck{Name: skill.Name + "/" + target.ID, Status: "warn", Detail: st.Message, Fix: "Run skills sync to repair a link that still points at the canonical skill."})
			case StatusConflict:
				checks = append(checks, DoctorCheck{Name: skill.Name + "/" + target.ID, Status: "warn", Detail: st.Message, Fix: "Move the unmanaged file aside, or leave it. Sync will not overwrite it."})
			case StatusDrifted:
				checks = append(checks, DoctorCheck{Name: skill.Name + "/" + target.ID, Status: "warn", Detail: st.Message, Fix: "Run skills sync. Only canonical-side edits are applied."})
			}
		}
	}
	for targetID, skillsMap := range m.state.Targets {
		for name, rec := range skillsMap {
			if _, ok := known[name]; !ok {
				checks = append(checks, DoctorCheck{
					Name: "orphan:" + targetID + "/" + name, Status: "warn",
					Detail: "state records " + rec.Path + " but the canonical skill is gone",
					Fix:    "Disable the target entry or restore the canonical skill. Disable will not delete a path whose identity does not match.",
				})
			}
		}
	}
	if len(checks) == 0 {
		checks = append(checks, DoctorCheck{Name: "deployments", Status: "ok", Detail: fmt.Sprintf("%d skills", len(skills))})
	}
	return checks
}
