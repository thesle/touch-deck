package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ButtonConfig represents a button's configuration
type ButtonConfig struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Command   string `json:"command"`
	BgImage   string `json:"bgImage"`
	BgColor   string `json:"bgColor"`
	FontColor string `json:"fontColor"`
	FontSize  int    `json:"fontSize"`
	Order     int    `json:"order"`
}

// PageConfig represents a page of buttons
type PageConfig struct {
	PageIndex int            `json:"pageIndex"`
	Buttons   []ButtonConfig `json:"buttons"`
}

// Config represents the application configuration
type Config struct {
	Rows  int          `json:"rows"`
	Cols  int          `json:"cols"`
	Pages []PageConfig `json:"pages"`
}

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetConfigPath returns the path to the config file
func (a *App) getConfigPath() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appConfigDir := filepath.Join(userConfigDir, "touchdeck")
	err = os.MkdirAll(appConfigDir, 0755)
	if err != nil {
		return "", err
	}
	return filepath.Join(appConfigDir, "config.json"), nil
}

// GetImagesDir returns the path to the custom images directory
func (a *App) getImagesDir() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	imagesDir := filepath.Join(userConfigDir, "touchdeck", "images")
	err = os.MkdirAll(imagesDir, 0755)
	if err != nil {
		return "", err
	}
	return imagesDir, nil
}

// LoadConfig loads the application configuration
func (a *App) LoadConfig() (Config, error) {
	configPath, err := a.getConfigPath()
	if err != nil {
		return Config{}, err
	}

	// If config doesn't exist, return a default one
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultConfig := Config{
			Rows: 2,
			Cols: 4,
			Pages: []PageConfig{
				{
					PageIndex: 0,
					Buttons: []ButtonConfig{
						{
							ID:      "btn_default_1",
							Label:   "Say Hello 👋",
							Command: "notify-send 'TouchDeck' 'Hello! Your TouchDeck is working beautifully.'",
							Order:   0,
						},
						{
							ID:      "btn_default_2",
							Label:   "Open GitHub 🌐",
							Command: "xdg-open 'https://github.com'",
							Order:   1,
						},
						{
							ID:      "btn_default_3",
							Label:   "System Date 🕒",
							Command: "notify-send 'Current Date' \"$(date)\"",
							Order:   2,
						},
					},
				},
			},
		}
		// Try to save the default config
		_ = a.SaveConfig(defaultConfig)
		return defaultConfig, nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	// Temporary structure to allow decoding older formats
	var raw struct {
		Rows    int `json:"rows"`
		Cols    int `json:"cols"`
		Buttons []struct {
			ButtonConfig
			Page int `json:"page"`
		} `json:"buttons"`
		Pages []PageConfig `json:"pages"`
	}

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&raw)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	cfg.Rows = raw.Rows
	cfg.Cols = raw.Cols

	if len(raw.Pages) > 0 {
		cfg.Pages = raw.Pages
	} else if len(raw.Buttons) > 0 {
		// Migrate old flat structure to nested Pages structure
		pagesMap := make(map[int][]ButtonConfig)
		for _, b := range raw.Buttons {
			btn := ButtonConfig{
				ID:        b.ID,
				Label:     b.Label,
				Command:   b.Command,
				BgImage:   b.BgImage,
				BgColor:   b.BgColor,
				FontColor: b.FontColor,
				Order:     b.Order,
			}
			pagesMap[b.Page] = append(pagesMap[b.Page], btn)
		}

		for pageIdx, btns := range pagesMap {
			cfg.Pages = append(cfg.Pages, PageConfig{
				PageIndex: pageIdx,
				Buttons:   btns,
			})
		}
	} else {
		cfg.Pages = []PageConfig{}
	}

	return cfg, nil
}

// SaveConfig saves the application configuration
func (a *App) SaveConfig(cfg Config) error {
	configPath, err := a.getConfigPath()
	if err != nil {
		return err
	}

	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cfg)
}

// RunCommandAsync executes a command in the background (fire-and-forget)
func (a *App) RunCommandAsync(command string) error {
	cmd := exec.Command("bash", "-c", command)
	err := cmd.Start()
	if err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

// RunCommandSync executes a command and waits for output (useful for testing scripts in Config)
func (a *App) RunCommandSync(command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after 10 seconds")
		}
		return string(output), err
	}
	return string(output), nil
}

// SelectImage opens a native file dialog to select an image
func (a *App) SelectImage() (string, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Background Image",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Images (*.png;*.jpg;*.jpeg;*.gif;*.webp;*.svg)",
				Pattern:     "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.svg",
			},
		},
	})
	if err != nil {
		return "", err
	}
	return filePath, nil
}

// CopyImageToConfig copies a selected image file to the touchdeck configs directory
func (a *App) CopyImageToConfig(srcPath string) (string, error) {
	if srcPath == "" {
		return "", nil
	}

	imagesDir, err := a.getImagesDir()
	if err != nil {
		return "", err
	}

	fileName := filepath.Base(srcPath)
	destFileName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), fileName)
	destPath := filepath.Join(imagesDir, destFileName)

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return "", err
	}

	return destPath, nil
}

// GetImageBase64 loads a local image file and returns its base64 representation
func (a *App) GetImageBase64(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	ext := filepath.Ext(path)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "image/png" // default fallback
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded), nil
}
