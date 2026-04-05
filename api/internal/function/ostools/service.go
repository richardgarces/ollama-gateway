package ostools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// blockedCommands contiene comandos peligrosos que nunca deben ejecutarse.
var blockedCommands = map[string]bool{
	"rm":       true,
	"rmdir":    true,
	"mkfs":     true,
	"dd":       true,
	"shutdown": true,
	"reboot":   true,
	"halt":     true,
	"poweroff": true,
	"init":     true,
	"kill":     true,
	"killall":  true,
	"pkill":    true,
	"chmod":    true,
	"chown":    true,
	"su":       true,
	"sudo":     true,
	"passwd":   true,
	"mount":    true,
	"umount":   true,
	"fdisk":    true,
	"format":   true,
	"curl":     true,
	"wget":     true,
	"nc":       true,
	"ncat":     true,
	"ssh":      true,
	"scp":      true,
	"rsync":    true,
	"ftp":      true,
	"sftp":     true,
	"telnet":   true,
}

// ExecResult contiene la salida de un comando ejecutado.
type ExecResult struct {
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	ExitCode int      `json:"exit_code"`
	Duration string   `json:"duration"`
}

// FileInfo contiene metadata de un archivo.
type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
	Mode    string `json:"mode"`
}

// Service gestiona operaciones del sistema operativo con restricción de seguridad.
type Service struct {
	repoRoot       string
	commandTimeout time.Duration
	maxOutputBytes int
	maxFileBytes   int64
	logger         *slog.Logger
}

func NewService(repoRoot string, logger *slog.Logger) (*Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("repo root inválido: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("repo root no es un directorio válido: %s", absRoot)
	}
	return &Service{
		repoRoot:       absRoot,
		commandTimeout: 30 * time.Second,
		maxOutputBytes: 1 << 20,  // 1 MB
		maxFileBytes:   10 << 20, // 10 MB
		logger:         logger,
	}, nil
}

// safePath valida que path esté dentro de repoRoot. Retorna el path absoluto.
func (s *Service) safePath(rawPath string) (string, error) {
	if rawPath == "" {
		return "", errors.New("path vacío")
	}
	// Resolver path relativo al repoRoot
	var target string
	if filepath.IsAbs(rawPath) {
		target = filepath.Clean(rawPath)
	} else {
		target = filepath.Clean(filepath.Join(s.repoRoot, rawPath))
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("path inválido: %w", err)
	}
	// Verificar que el path resida dentro de repoRoot
	if !strings.HasPrefix(abs, s.repoRoot+string(os.PathSeparator)) && abs != s.repoRoot {
		return "", fmt.Errorf("acceso denegado: path fuera de repo root (%s)", s.repoRoot)
	}
	return abs, nil
}

// ReadFile lee un archivo dentro del repoRoot.
func (s *Service) ReadFile(_ context.Context, rawPath string) (string, error) {
	abs, err := s.safePath(rawPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("archivo no encontrado: %w", err)
	}
	if info.IsDir() {
		return "", errors.New("el path es un directorio, usa ListDir")
	}
	if info.Size() > s.maxFileBytes {
		return "", fmt.Errorf("archivo demasiado grande (%d bytes, max %d)", info.Size(), s.maxFileBytes)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("error leyendo archivo: %w", err)
	}
	return string(data), nil
}

// WriteFile escribe contenido en un archivo dentro del repoRoot.
func (s *Service) WriteFile(_ context.Context, rawPath, content string) error {
	abs, err := s.safePath(rawPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("error creando directorio: %w", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return fmt.Errorf("error escribiendo archivo: %w", err)
	}
	s.logger.Info("archivo escrito",
		slog.String("path", abs),
		slog.Int("bytes", len(content)),
	)
	return nil
}

// ListDir lista contenido de un directorio dentro del repoRoot.
func (s *Service) ListDir(_ context.Context, rawPath string) ([]FileInfo, error) {
	abs, err := s.safePath(rawPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("directorio no encontrado: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("el path no es un directorio")
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("error leyendo directorio: %w", err)
	}
	result := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(s.repoRoot, filepath.Join(abs, e.Name()))
		result = append(result, FileInfo{
			Name:    e.Name(),
			Path:    rel,
			Size:    fi.Size(),
			IsDir:   fi.IsDir(),
			ModTime: fi.ModTime().Format(time.RFC3339),
			Mode:    fi.Mode().String(),
		})
	}
	return result, nil
}

// DeleteFile elimina un archivo dentro del repoRoot. No elimina directorios.
func (s *Service) DeleteFile(_ context.Context, rawPath string) error {
	abs, err := s.safePath(rawPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("archivo no encontrado: %w", err)
	}
	if info.IsDir() {
		return errors.New("no se puede eliminar directorios por seguridad; usa el comando adecuado")
	}
	if err := os.Remove(abs); err != nil {
		return fmt.Errorf("error eliminando archivo: %w", err)
	}
	s.logger.Info("archivo eliminado", slog.String("path", abs))
	return nil
}

// FileExists verifica si un archivo o directorio existe dentro del repoRoot.
func (s *Service) FileExists(_ context.Context, rawPath string) (bool, bool, error) {
	abs, err := s.safePath(rawPath)
	if err != nil {
		return false, false, err
	}
	info, err := os.Stat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, info.IsDir(), nil
}

// Exec ejecuta un comando del sistema dentro del repoRoot como working directory.
func (s *Service) Exec(ctx context.Context, command string, args []string) (*ExecResult, error) {
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("comando vacío")
	}
	// Extraer nombre base del comando (sin path)
	baseName := filepath.Base(command)
	if blockedCommands[baseName] {
		return nil, fmt.Errorf("comando bloqueado por seguridad: %s", baseName)
	}
	// Sanitizar argumentos: rechazar inyección de shell
	for _, arg := range args {
		if strings.ContainsAny(arg, "|;&$`\\\"'(){}!<>") {
			return nil, fmt.Errorf("argumento contiene caracteres de shell no permitidos: %q", arg)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, s.commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = s.repoRoot
	// Heredar variables de entorno del proceso pero sin expandir shell
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)
	exitCode := 0

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("error ejecutando comando: %w", err)
		}
	}

	outStr := stdout.String()
	errStr := stderr.String()
	if len(outStr) > s.maxOutputBytes {
		outStr = outStr[:s.maxOutputBytes] + "\n... (truncado)"
	}
	if len(errStr) > s.maxOutputBytes {
		errStr = errStr[:s.maxOutputBytes] + "\n... (truncado)"
	}

	s.logger.Info("comando ejecutado",
		slog.String("command", command),
		slog.Any("args", args),
		slog.Int("exit_code", exitCode),
		slog.String("duration", duration.String()),
	)

	return &ExecResult{
		Command:  command,
		Args:     args,
		Stdout:   outStr,
		Stderr:   errStr,
		ExitCode: exitCode,
		Duration: duration.String(),
	}, nil
}
