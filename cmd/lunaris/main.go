package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/slide/LunarisInjector/pkg/config"
	"github.com/slide/LunarisInjector/pkg/injector"
	"github.com/slide/LunarisInjector/pkg/installer"
	"github.com/slide/LunarisInjector/pkg/manifest"
	"github.com/slide/LunarisInjector/pkg/server"
	"github.com/slide/LunarisInjector/pkg/syncer"
)

const Version = "1.0.0"

func main() {
	args := os.Args[1:]

	// If invoked with Minecraft / Java launch arguments, run the injector directly
	if isJavaInvocation(args) {
		runInjector(args)
		return
	}

	// Otherwise, handle CLI subcommands
	if len(args) == 0 {
		printUsage()
		return
	}

	switch args[0] {
	case "run", "inject":
		runInjector(args[1:])
	case "install":
		cmdInstall(args[1:])
	case "uninstall":
		cmdUninstall(args[1:])
	case "server", "serve":
		cmdServer(args[1:])
	case "generate", "gen":
		cmdGenerate(args[1:])
	case "verify", "check":
		cmdVerify(args[1:])
	case "instances", "list":
		cmdListInstances()
	case "version", "-v", "--version":
		fmt.Printf("LunarisInjector v%s\n", Version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

// isJavaInvocation determines whether the process was executed by a Minecraft launcher
// as a Java executable replacement.
func isJavaInvocation(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-Xmx") ||
			strings.HasPrefix(arg, "-Xms") ||
			strings.HasPrefix(arg, "-Djava.library.path") ||
			strings.HasPrefix(arg, "-Dminecraft.applet.TargetDirectory") ||
			strings.HasPrefix(arg, "--gameDir") ||
			arg == "-cp" || arg == "-classpath" ||
			strings.Contains(arg, "net.minecraft.") ||
			strings.Contains(arg, "bootstraplauncher") ||
			strings.Contains(arg, "fabricmc") {
			return true
		}
	}
	return false
}

// runInjector intercepts the launch, synchronizes mods, then executes real Java.
func runInjector(args []string) {
	fmt.Println("==================================================")
	fmt.Printf(" [LunarisInjector v%s] Pre-launch Synchronizer\n", Version)
	fmt.Println("==================================================")

	gameDir := injector.ExtractGameDir(args)
	if gameDir == "" {
		cwd, _ := os.Getwd()
		gameDir = cwd
	}
	fmt.Printf("[Lunaris] Game Directory: %s\n", gameDir)

	cfg, cfgPath, err := config.FindInstanceConfig(gameDir)
	if err != nil {
		fmt.Println("[Lunaris] Warning: No lunaris.json configuration found.")
		fmt.Println("[Lunaris] Skipping file sync and launching Minecraft directly.")
	} else {
		fmt.Printf("[Lunaris] Loaded config from: %s\n", cfgPath)
		s := syncer.New(syncer.SyncOptions{
			Config:      cfg,
			GameDir:     gameDir,
			WorkerCount: 4,
			Logger: func(format string, a ...interface{}) {
				fmt.Printf(format+"\n", a...)
			},
		})

		ctx := context.Background()
		if err := s.Sync(ctx); err != nil && err != syncer.ErrOfflineProceed {
			fmt.Printf("[Lunaris] Sync failed: %v\n", err)
			if !cfg.OfflineLaunch {
				fmt.Println("[Lunaris] Offline launch is disabled. Aborting startup.")
				os.Exit(1)
			}
		}
	}

	// Locate real Java
	configuredJava := ""
	if cfg != nil {
		configuredJava = cfg.RealJavaPath
	}

	realJava, err := injector.FindRealJava(configuredJava)
	if err != nil {
		fmt.Printf("[Lunaris] FATAL ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[Lunaris] Launching Minecraft via: %s\n", realJava)
	fmt.Println("==================================================")

	exitCode, err := injector.RunJava(realJava, args)
	if err != nil {
		fmt.Printf("[Lunaris] Error running Java: %v\n", err)
	}
	os.Exit(exitCode)
}

func cmdInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	instanceFlag := fs.String("instance", "", "Path to the Minecraft instance directory")
	serverFlag := fs.String("server", "", "Sync server URL (e.g. http://192.168.1.100:8080)")
	javaFlag := fs.String("java", "", "Path to the real Java binary (auto-detected if empty)")
	profileFile := fs.String("profile-file", "", "Path to launcher_profiles.json (optional)")
	profileID := fs.String("profile-id", "", "Profile ID in launcher_profiles.json (optional)")
	_ = fs.Parse(args)

	// Non-interactive mode
	if *instanceFlag != "" && *serverFlag != "" {
		err := installer.Install(installer.InstallConfig{
			InstanceDir:   *instanceFlag,
			ServerURL:     *serverFlag,
			RealJavaPath:  *javaFlag,
			ProfileFile:   *profileFile,
			ProfileID:     *profileID,
		})
		if err != nil {
			fmt.Printf("Installation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ LunarisInjector successfully configured for:", *instanceFlag)
		return
	}

	// Interactive mode
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("=== LunarisInjector Auto-Installer ===")
	fmt.Println("Scanning for installed Minecraft modpacks & profiles...")

	detected := installer.DetectInstances()
	var selectedPath string
	var selectedProfileFile string
	var selectedProfileID string

	if len(detected) > 0 {
		fmt.Println("\nDetected instances:")
		for i, inst := range detected {
			status := ""
			if inst.IsInjected {
				status = fmt.Sprintf(" [ALREADY INJECTED: %s]", inst.ConfiguredServer)
			}
			fmt.Printf(" [%d] [%s] %s%s\n     Path: %s\n", i+1, inst.Launcher, inst.Name, status, inst.Path)
		}
		fmt.Printf(" [%d] Enter custom instance folder path\n", len(detected)+1)

		for {
			fmt.Printf("\nSelect an instance [1-%d]: ", len(detected)+1)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			idx, err := strconv.Atoi(input)
			if err == nil && idx >= 1 && idx <= len(detected) {
				sel := detected[idx-1]
				selectedPath = sel.Path
				selectedProfileFile = sel.ProfileFile
				selectedProfileID = sel.ProfileID
				break
			} else if err == nil && idx == len(detected)+1 {
				break
			}
			fmt.Println("Invalid selection. Try again.")
		}
	}

	if selectedPath == "" {
		fmt.Print("\nEnter instance directory (where 'mods' folder lives): ")
		input, _ := reader.ReadString('\n')
		selectedPath = strings.TrimSpace(input)
	}

	if selectedPath == "" {
		fmt.Println("No instance directory selected. Aborting.")
		return
	}

	// Server URL
	defaultServer := "http://localhost:8080"
	fmt.Printf("\nEnter Sync Server URL (default: %s): ", defaultServer)
	serverInput, _ := reader.ReadString('\n')
	serverInput = strings.TrimSpace(serverInput)
	if serverInput == "" {
		serverInput = defaultServer
	}

	// Real Java detection
	detectedJava, _ := injector.FindRealJava("")
	fmt.Printf("\nEnter Real Java path (press Enter for auto-detected: %s): ", detectedJava)
	javaInput, _ := reader.ReadString('\n')
	javaInput = strings.TrimSpace(javaInput)
	if javaInput == "" {
		javaInput = detectedJava
	}

	selfExe, _ := os.Executable()
	selfExe, _ = filepath.Abs(selfExe)

	fmt.Println("\nInstalling LunarisInjector...")
	err := installer.Install(installer.InstallConfig{
		InstanceDir:   selectedPath,
		ServerURL:     serverInput,
		RealJavaPath:  javaInput,
		LunarisBinary: selfExe,
		ProfileFile:   selectedProfileFile,
		ProfileID:     selectedProfileID,
	})

	if err != nil {
		fmt.Printf("Installation error: %v\n", err)
		return
	}

	fmt.Println("\n=======================================================")
	fmt.Println("✓ Installation Complete!")
	fmt.Println("=======================================================")
	fmt.Printf("Instance Directory : %s\n", selectedPath)
	fmt.Printf("Sync Server URL    : %s\n", serverInput)
	fmt.Printf("Config File        : %s/lunaris.json\n", selectedPath)
	fmt.Printf("Injector Executable: %s\n", selfExe)
	fmt.Println("\nHow it works:")
	fmt.Println("1. Whenever Minecraft is launched for this instance, LunarisInjector")
	fmt.Println("   runs first, synchronizes mods from the server, and launches Minecraft.")
	fmt.Println("2. In CurseForge, ensure the profile's 'Java Executable' or the launcher's")
	fmt.Printf("   javaDir is pointed to: %s\n", selfExe)
	fmt.Println("=======================================================")
}

func cmdUninstall(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	instanceFlag := fs.String("instance", "", "Path to the Minecraft instance directory")
	_ = fs.Parse(args)

	target := *instanceFlag
	if target == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter instance directory to uninstall Lunaris from: ")
		input, _ := reader.ReadString('\n')
		target = strings.TrimSpace(input)
	}

	if target == "" {
		fmt.Println("No directory provided.")
		return
	}

	err := installer.Uninstall(target, "", "")
	if err != nil {
		fmt.Printf("Uninstall error: %v\n", err)
		return
	}
	fmt.Println("✓ LunarisInjector uninstalled from:", target)
}

func cmdServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	dirFlag := fs.String("dir", ".", "Directory to host and sync (e.g. your modpack folder)")
	portFlag := fs.Int("port", 8080, "Port to listen on")
	hostFlag := fs.String("host", "0.0.0.0", "Host address to bind to")
	_ = fs.Parse(args)

	absDir, err := filepath.Abs(*dirFlag)
	if err != nil {
		fmt.Printf("Invalid directory: %v\n", err)
		os.Exit(1)
	}

	// Ensure mods folder exists if dir is empty
	_ = os.MkdirAll(filepath.Join(absDir, "mods"), 0755)

	srv := server.New(server.ServerOptions{
		RootDir:  absDir,
		SyncDirs: []string{"mods", "config"},
		Port:     *portFlag,
		Host:     *hostFlag,
	})

	if err := srv.Start(); err != nil {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
	}
}

func cmdGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	dirFlag := fs.String("dir", ".", "Directory containing files to scan")
	outFlag := fs.String("out", "", "Output path for manifest.json (default: <dir>/manifest.json)")
	_ = fs.Parse(args)

	absDir, err := filepath.Abs(*dirFlag)
	if err != nil {
		fmt.Printf("Invalid directory: %v\n", err)
		os.Exit(1)
	}

	outFile := *outFlag
	if outFile == "" {
		outFile = filepath.Join(absDir, "manifest.json")
	}

	fmt.Printf("Scanning '%s' and generating manifest...\n", absDir)
	m, err := manifest.ScanDirectory(absDir, []string{"mods", "config"}, nil)
	if err != nil {
		fmt.Printf("Scan error: %v\n", err)
		os.Exit(1)
	}

	if err := m.SaveToFile(outFile); err != nil {
		fmt.Printf("Failed to save manifest: %v\n", err)
		os.Exit(1)
	}

	var totalBytes int64
	for _, f := range m.Files {
		totalBytes += f.Size
	}

	fmt.Println("✓ Manifest generated successfully!")
	fmt.Printf("  Output file : %s\n", outFile)
	fmt.Printf("  Files tracked: %d\n", len(m.Files))
	fmt.Printf("  Total size  : %.2f MB\n", float64(totalBytes)/(1024*1024))
}

func cmdVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	instanceFlag := fs.String("instance", ".", "Path to the Minecraft instance directory")
	serverFlag := fs.String("server", "", "Sync server URL (optional, reads from lunaris.json if omitted)")
	_ = fs.Parse(args)

	absDir, err := filepath.Abs(*instanceFlag)
	if err != nil {
		fmt.Printf("Invalid directory: %v\n", err)
		os.Exit(1)
	}

	cfg, _, err := config.FindInstanceConfig(absDir)
	if err != nil && *serverFlag == "" {
		fmt.Println("Error: No lunaris.json found and no --server URL specified.")
		os.Exit(1)
	}

	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if *serverFlag != "" {
		cfg.ServerURL = *serverFlag
	}

	s := syncer.New(syncer.SyncOptions{
		Config:      cfg,
		GameDir:     absDir,
		WorkerCount: 4,
		Logger: func(format string, a ...interface{}) {
			fmt.Printf(format+"\n", a...)
		},
	})

	remote, err := s.FetchRemoteManifest(context.Background())
	if err != nil {
		fmt.Printf("Failed to fetch remote manifest: %v\n", err)
		os.Exit(1)
	}

	plan, err := s.CalculatePlan(remote)
	if err != nil {
		fmt.Printf("Failed to calculate plan: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n=== Verification Report ===")
	fmt.Printf("Matches / In Sync : %d files\n", plan.Unchanged)
	fmt.Printf("Need Download     : %d files (%.2f MB)\n", len(plan.Downloads), float64(plan.TotalBytes)/(1024*1024))
	for _, d := range plan.Downloads {
		fmt.Printf("  + [ADD/UPDATE] %s (%d bytes)\n", d.Path, d.Size)
	}
	fmt.Printf("Obsolete / Delete : %d files\n", len(plan.Deletions))
	for _, del := range plan.Deletions {
		fmt.Printf("  - [DELETE]     %s\n", del)
	}

	if len(plan.Downloads) == 0 && len(plan.Deletions) == 0 {
		fmt.Println("\n✓ Instance is 100% in sync with the remote server!")
	} else {
		fmt.Println("\n! Instance has discrepancies compared to the remote server.")
	}
}

func cmdListInstances() {
	fmt.Println("Scanning for installed Minecraft instances...")
	detected := installer.DetectInstances()
	if len(detected) == 0 {
		fmt.Println("No instances automatically detected.")
		return
	}

	fmt.Printf("Found %d instances:\n\n", len(detected))
	for i, inst := range detected {
		injected := "No"
		if inst.IsInjected {
			injected = fmt.Sprintf("Yes (Syncing with: %s)", inst.ConfiguredServer)
		}
		fmt.Printf("[%d] %s (%s)\n", i+1, inst.Name, inst.Launcher)
		fmt.Printf("    Path    : %s\n", inst.Path)
		fmt.Printf("    Injected: %s\n", injected)
		if inst.CurrentJava != "" {
			fmt.Printf("    Java    : %s\n", inst.CurrentJava)
		}
		fmt.Println()
	}
}

func printUsage() {
	fmt.Printf(`LunarisInjector v%s - Minecraft Modpack Auto-Synchronizer & Launch Wrapper

USAGE:
  lunaris <command> [arguments]
  OR configure lunaris as the Java Executable in your launcher / CurseForge instance.

COMMANDS:
  install       Interactive or CLI setup to hook an instance into Lunaris
  uninstall     Revert an instance back to normal
  server        Run a built-in sync server with live manifest & file downloads
  generate      Generate a static manifest.json from a folder (for Nginx, S3, etc.)
  verify        Check an instance's mods against the server without launching
  instances     List all detected CurseForge, Vanilla, and Prism instances
  run           Manually test the injector wrapper on a game directory
  version       Print version information

EXAMPLES:
  # 1. Run the interactive installer
  lunaris install

  # 2. Host a sync server for your friends/players from your modpack folder
  lunaris server --dir ./my-modpack --port 8080

  # 3. Generate static manifest for hosting on Nginx or GitHub / S3
  lunaris generate --dir ./my-modpack

  # 4. Check instance sync status
  lunaris verify --instance ~/.minecraft

`, Version)
}
