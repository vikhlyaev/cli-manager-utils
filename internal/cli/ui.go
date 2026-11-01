package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"cli-manager/internal/webui"
)

// runUI поднимает локальный веб-интерфейс на 127.0.0.1 и открывает браузер.
// Работает до Ctrl+C.
func runUI(args []string, s IO) int {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	fs.SetOutput(s.Err)
	port := fs.Int("port", 0, "порт (0 — любой свободный)")
	noOpen := fs.Bool("no-open", false, "не открывать браузер, только напечатать адрес")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	srv, err := webui.New()
	if err != nil {
		fmt.Fprintln(s.Err, "cli-manager ui:", err)
		return 1
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)))
	if err != nil {
		fmt.Fprintln(s.Err, "cli-manager ui:", err)
		return 1
	}
	srv.SetAddr(ln.Addr().String())

	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()

	fmt.Fprintln(s.Out, "Веб-интерфейс:", srv.URL())
	fmt.Fprintln(s.Out, "Остановить — Ctrl+C")
	if !*noOpen {
		if err := openBrowser(srv.URL()); err != nil {
			fmt.Fprintln(s.Err, "Не удалось открыть браузер — откройте адрес вручную:", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(s.Err, "cli-manager ui:", err)
			return 1
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = hs.Shutdown(shutdownCtx)
	fmt.Fprintln(s.Out, "\nВеб-интерфейс остановлен")
	return 0
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
