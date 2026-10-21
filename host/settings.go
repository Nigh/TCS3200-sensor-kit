package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type settings struct {
	Version                    int
	Low                        uint32
	Channels                   byte
	Tab, Format, Width, Height int
	Enter, Exit, Delay         string
	Reference                  uint32
	ReferenceChannel           byte
	ReferenceLow               uint32
	Calibration                *calibrationMap
}

func defaultSettings() settings {
	return settings{Version: 1, Low: 15000, Channels: 15, Width: 1280, Height: 900, Enter: "10", Exit: "30", Delay: "0"}
}
func readSettings(path string) (settings, error) {
	s := defaultSettings()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(data, &s); err != nil {
		return defaultSettings(), err
	}
	if s.Version != 1 || s.Low < 100 || s.Low > 600000 || s.Channels > 15 || s.Tab < 0 || s.Tab > 2 || s.Format < 0 || s.Format > 2 || s.Width < 1080 || s.Height < 760 || s.Width > 16384 || s.Height > 16384 {
		return defaultSettings(), errors.New("设置版本或数值不受支持")
	}
	if s.Calibration != nil {
		if err = s.Calibration.validate(); err != nil {
			return defaultSettings(), err
		}
	}
	return s, nil
}
func writeSettings(path string, s settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func (s *app) loadSettings() {
	dir, err := os.UserConfigDir()
	if err != nil {
		s.settingsBlocked = true
		s.settingsError = "无法找到设置目录：" + err.Error()
		return
	}
	s.settingsPath = filepath.Join(dir, "tcs3200-host", "settings.json")
	p, err := readSettings(s.settingsPath)
	s.prefs = p
	if err != nil {
		s.settingsBlocked = true
		s.settingsError = "设置文件未加载，已保留原文件：" + err.Error()
		return
	}
	s.lowInput = fmt.Sprint(p.Low)
	s.tab = p.Tab
	s.colorFormat = p.Format
	s.enterInput = p.Enter
	s.exitInput = p.Exit
	s.delayInput = p.Delay
	s.reference = p.Reference
	s.referenceChannel = p.ReferenceChannel
	s.referenceLow = p.ReferenceLow
	s.calibration = p.Calibration
	s.settingsLoaded = true
}
func (s *app) savePreferences() error {
	if s.settingsPath == "" || s.demo {
		return nil
	}
	if s.settingsBlocked {
		return errors.New("原设置文件不可读，请手动重置全部设置后再保存")
	}
	p := s.prefs
	p.Version = 1
	p.Tab = s.tab
	p.Format = s.colorFormat
	p.Enter = s.enterInput
	p.Exit = s.exitInput
	p.Delay = s.delayInput
	p.Reference = s.reference
	p.ReferenceChannel = s.referenceChannel
	p.ReferenceLow = s.referenceLow
	p.Calibration = s.calibration
	data, _ := json.Marshal(p)
	if string(data) == s.savedSettings {
		return nil
	}
	if err := writeSettings(s.settingsPath, p); err != nil {
		s.settingsError = "保存失败：" + err.Error()
		return err
	}
	s.prefs = p
	s.savedSettings = string(data)
	s.settingsError = ""
	return nil
}
func (s *app) resetPreferences(all bool) {
	if all {
		p := defaultSettings()
		if s.settingsPath != "" {
			if err := writeSettings(s.settingsPath, p); err != nil {
				s.settingsError = "重置失败：" + err.Error()
				return
			}
		}
		s.prefs = p
		s.settingsBlocked = false
		s.settingsLoaded = true
		s.calibration = nil
		s.tab = 0
		s.colorFormat = 0
		s.enterInput = p.Enter
		s.exitInput = p.Exit
		s.delayInput = p.Delay
		s.reference = 0
		s.referenceValid = false
		s.lowInput = "15000"
		s.savedSettings = ""
		s.backend.applyAcquisition(15000, 15)
	} else {
		old := s.calibration
		s.calibration = nil
		if err := s.savePreferences(); err != nil {
			s.calibration = old
			return
		}
	}
	s.resetConfirm = ""
}
