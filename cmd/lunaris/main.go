package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/engineeringjack/LunarisInjector/pkg/config"
	"github.com/engineeringjack/LunarisInjector/pkg/gui"
	"github.com/engineeringjack/LunarisInjector/pkg/injector"
	"github.com/engineeringjack/LunarisInjector/pkg/installer"
	"github.com/engineeringjack/LunarisInjector/pkg/logger"
	"github.com/engineeringjack/LunarisInjector/pkg/manifest"
	"github.com/engineeringjack/LunarisInjector/pkg/modtracker"
	"github.com/engineeringjack/LunarisInjector/pkg/server"
	"github.com/engineeringjack/LunarisInjector/pkg/syncer"
	"github.com/engineeringjack/LunarisInjector/pkg/updater"
)

var Version = config.Version

func main() {
	var cleanArgs []string
	for _, a := range os.Args[1:] {
		// Ignore macOS Finder Process Serial Number argument (e.g. -psn_0_123456)
		if strings.HasPrefix(a, "-psn") {
			continue
		}
		cleanArgs = append(cleanArgs, a)
	}
	args := cleanArgs

	siblingRealJava := injector.GetSiblingRealJava()

	// If running as a hooked Java wrapper (sibling *.real exists in the same directory):
	if siblingRealJava != "" {
		// 1. If explicit administrative CLI command was passed (e.g. `java install`, `java uninstall`):
		if len(args) > 0 && isAdministrativeCommand(args[0]) {
			handleAdminCommand(args)
			return
		}

		// 2. Check if this is a Minecraft launch targeting a Lunaris instance:
		if isMinecraftLaunch(args) {
			gameDir := injector.ExtractGameDir(args)
			if gameDir != "" && config.IsLunarisInstance(gameDir) {
				// Target instance is a Lunaris instance! Run sync then launch real Java:
				runInjector(args, siblingRealJava)
				return
			}
		}

		// 3. For any other call (e.g. `java -version`, other non-Lunaris modpack launch, other tools):
		// Transparently and silently pass through directly to real Java!
		exitCode, err := injector.ExecOrRunJava(siblingRealJava, args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to execute Java runtime: %v\n", err)
		}
		os.Exit(exitCode)
		return
	}

	// Standalone CLI invocation:
	if isMinecraftLaunch(args) || isJavaInvocation(args) {
		runInjector(args, "")
		return
	}

	if len(args) == 0 {
		cmdGUI(nil)
		return
	}

	handleAdminCommand(args)
}

// isAdministrativeCommand checks if the first argument is a Lunaris CLI subcommand.
func isAdministrativeCommand(cmd string) bool {
	switch strings.ToLower(cmd) {
	case "gui", "install", "uninstall", "update", "upgrade", "server", "serve", "generate", "gen", "verify", "check", "instances", "list", "resync", "reset":
		return true
	default:
		return false
	}
}

// isMinecraftLaunch detects if arguments correspond to a Minecraft client launch.
func isMinecraftLaunch(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--gameDir" || strings.HasPrefix(arg, "--gameDir=") {
			return true
		}
		if strings.HasPrefix(arg, "-Dminecraft.applet.TargetDirectory=") {
			return true
		}
		if strings.Contains(arg, "net.minecraft.") ||
			strings.Contains(arg, "cpw.mods.bootstraplauncher.") ||
			strings.Contains(arg, "net.minecraftforge.") ||
			strings.Contains(arg, "fabricmc") ||
			strings.Contains(arg, "quiltmc") {
			return true
		}
	}
	return false
}

func handleAdminCommand(args []string) {
	switch args[0] {
	case "gui":
		cmdGUI(args[1:])
	case "run", "inject":
		runInjector(args[1:], "")
	case "install":
		cmdInstall(args[1:])
	case "uninstall":
		cmdUninstall(args[1:])
	case "resync", "reset":
		cmdResync(args[1:])
	case "update", "upgrade":
		cmdUpdate(args[1:])
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
func runInjector(args []string, siblingRealJava string) {
	gameDir := injector.ExtractGameDir(args)
	if gameDir == "" {
		cwd, _ := os.Getwd()
		gameDir = cwd
	}

	// Try loading instance config early to check for custom LogFile setting
	cfg, cfgPath, cfgErr := config.FindInstanceConfig(gameDir)

	var logOpts logger.Options
	logOpts.TargetDir = gameDir
	logOpts.Version = Version
	logOpts.Args = args
	if cfg != nil && cfg.LogFile != "" {
		if filepath.IsAbs(cfg.LogFile) {
			logOpts.CustomLogFile = cfg.LogFile
		} else {
			logOpts.CustomLogFile = filepath.Join(gameDir, cfg.LogFile)
		}
	}

	// Initialize action logging for gameDir (overwrites or removes previous run's log)
	logFiles, logErr := logger.Init(gameDir, logOpts)
	if logErr != nil {
		fmt.Fprintf(os.Stderr, "[Lunaris] Warning: Failed to initialize log file: %v\n", logErr)
	}
	defer logger.Close()

	logger.Infof("==================================================")
	logger.Infof(" [LunarisInjector v%s] Pre-launch Synchronizer", Version)
	logger.Infof("==================================================")
	if len(logFiles) > 0 {
		logger.Infof("[Lunaris] Action log initialized: %s", strings.Join(logFiles, ", "))
	}
	logger.Infof("[Lunaris] Game Directory: %s", gameDir)
	if siblingRealJava != "" {
		logger.Infof("[Lunaris] Operating Mode: Hooked CurseForge Java Wrapper (%s)", siblingRealJava)
	} else {
		logger.Infof("[Lunaris] Operating Mode: Standalone Launch Interceptor")
	}

	updater.CleanupOld()

	if cfgErr != nil {
		logger.Warnf("[Lunaris] Warning: No lunaris.json configuration found (%v).", cfgErr)
		logger.Warnf("[Lunaris] Skipping file sync and launching Minecraft directly.")
	} else {
		logger.Infof("[Lunaris] Loaded config from: %s", cfgPath)
		logger.Infof("[Lunaris] Sync Server: %s | VR Mode: %v | Delete Extra: %v", cfg.ServerURL, cfg.EnableVR, cfg.DeleteExtra)

		// Check for auto-update if enabled
		if cfg.AutoUpdate {
			repo := cfg.GitHubRepo
			if repo == "" {
				repo = updater.DefaultGitHubRepo
			}
			logger.Infof("[Lunaris] Checking GitHub repo for auto-updates: %s", repo)
			updateCtx, updateCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, _ = updater.AutoCheckAndUpdate(updateCtx, repo, Version, gameDir, false, logger.AsFunc())
			updateCancel()
		}

		// Version Check: ensure game version matches requirement
		detectedVer := injector.ExtractGameVersion(args, gameDir)
		if detectedVer != "" {
			logger.Infof("[Lunaris] Detected Minecraft Version: %s (Configured target: %s)", detectedVer, cfg.GameVersion)
		}
		if detectedVer != "" && cfg.GameVersion != "" && detectedVer != cfg.GameVersion {
			logger.Errorf("==================================================")
			logger.Errorf("[Lunaris] FATAL ERROR: Minecraft version mismatch!")
			logger.Errorf("[Lunaris] Expected: Minecraft %s | Instance: Minecraft %s", cfg.GameVersion, detectedVer)
			logger.Errorf("[Lunaris] Aborting sync to prevent corrupting incompatible game files.")
			logger.Errorf("==================================================")
			_ = logger.Sync()
			_ = logger.Close()
			os.Exit(1)
		}

		// Load existing baseline state if present
		state, stateErr := modtracker.LoadState(gameDir)

		isCLI := false
		for _, a := range args {
			if a == "--cli" {
				isCLI = true
				break
			}
		}

		var initialRules *modtracker.UserRules
		if state != nil {
			initialRules = &state.UserRules
		}

		s := syncer.New(syncer.SyncOptions{
			Config:      cfg,
			GameDir:     gameDir,
			WorkerCount: 4,
			Logger:      logger.AsFunc(),
			UserRules:   initialRules,
		})

		ctx := context.Background()
		var features []string
		if cfg.EnableVR {
			features = append(features, "vr")
		}

		remoteM, err := s.FetchRemoteManifest(ctx)
		if err != nil {
			if cfg.OfflineLaunch {
				logger.Warnf("[Lunaris] Remote sync server is offline or unreachable (%v).", err)
				logger.Infof("[Lunaris] Offline launch enabled. Starting game with existing local files.")
				if state != nil {
					// Detect local changes against baseline even when offline
					offlineMap := make(map[string]manifest.FileEntry, len(state.BaselineFiles))
					for p, fs := range state.BaselineFiles {
						offlineMap[p] = manifest.FileEntry{
							Path:   p,
							SHA256: fs.SHA256,
							Size:   fs.Size,
						}
					}
					changes, _ := modtracker.DetectChanges(gameDir, state, offlineMap, cfg.SyncDirs, cfg.IgnoreFiles)
					if len(changes) > 0 {
						logger.Infof("[Lunaris] %d modpack modification(s) detected since last launch. Requesting user action...", len(changes))
						decisions, cancelled, pErr := gui.ShowChangePrompt(gui.PromptOptions{
							InstanceDir:  gameDir,
							InstanceName: filepath.Base(gameDir),
							Changes:      changes,
							IsCLI:        isCLI,
							Logger:       logger.AsFunc(),
						})
						if cancelled {
							logger.Infof("[Lunaris] Launch cancelled by user during mod change review.")
							_ = logger.Sync()
							_ = logger.Close()
							os.Exit(0)
						}
						if pErr == nil && decisions != nil {
							_ = modtracker.ApplyDecisions(gameDir, state, decisions, changes)
						}
					}
				}
			} else {
				logger.Errorf("[Lunaris] Sync server unreachable: %v", err)
				logger.Errorf("[Lunaris] Offline launch is disabled. Aborting startup.")
				_ = logger.Sync()
				_ = logger.Close()
				os.Exit(1)
			}
		} else {
			remoteMap := remoteM.FilteredMap(features)

			if stateErr == modtracker.ErrNoState {
				// First run: Establish baseline without prompting the user
				logger.Infof("[Lunaris] First run detected: Establishing baseline sync with server modpack.")
				if err := s.SyncWithManifest(ctx, remoteM); err != nil {
					logger.Errorf("[Lunaris] Initial sync failed: %v", err)
				} else {
					newState, bErr := modtracker.CreateInitialBaseline(gameDir, remoteMap)
					if bErr != nil {
						logger.Warnf("[Lunaris] Warning: Failed to save initial baseline: %v", bErr)
					} else {
						logger.Infof("[Lunaris] Initial modpack baseline established (%d files tracked).", len(newState.BaselineFiles))
					}
				}
			} else if state != nil {
				// Subsequent run: Check if user made changes to mods or configs
				changes, detectErr := modtracker.DetectChanges(gameDir, state, remoteMap, cfg.SyncDirs, cfg.IgnoreFiles)
				if detectErr != nil {
					logger.Warnf("[Lunaris] Warning: Failed to scan directory for mod changes: %v", detectErr)
				}

				if len(changes) > 0 {
					logger.Infof("[Lunaris] %d modpack modification(s) detected since last launch. Requesting user action...", len(changes))
					decisions, cancelled, pErr := gui.ShowChangePrompt(gui.PromptOptions{
						InstanceDir:  gameDir,
						InstanceName: filepath.Base(gameDir),
						Changes:      changes,
						IsCLI:        isCLI,
						Logger:       logger.AsFunc(),
					})
					if cancelled {
						logger.Infof("[Lunaris] Launch cancelled by user during mod change review.")
						_ = logger.Sync()
						_ = logger.Close()
						os.Exit(0)
					}
					if pErr != nil {
						logger.Warnf("[Lunaris] Warning during change prompt: %v", pErr)
					}
					if err := modtracker.ApplyDecisions(gameDir, state, decisions, changes); err != nil {
						logger.Errorf("[Lunaris] Failed to apply user decisions: %v", err)
					}
				}

				// Apply current user rules to the syncer
				s.SetUserRules(&state.UserRules)

				if err := s.SyncWithManifest(ctx, remoteM); err != nil {
					logger.Errorf("[Lunaris] Sync failed: %v", err)
					if !cfg.OfflineLaunch {
						logger.Errorf("[Lunaris] Offline launch is disabled. Aborting startup.")
						_ = logger.Sync()
						_ = logger.Close()
						os.Exit(1)
					}
				} else {
					// Refresh baseline files with server manifest
					if bErr := modtracker.UpdateBaseline(gameDir, state, remoteMap); bErr != nil {
						logger.Warnf("[Lunaris] Warning: Failed to update baseline snapshot: %v", bErr)
					}
				}
			}
		}
	}

	// Locate real Java
	realJava := siblingRealJava
	if realJava == "" {
		configuredJava := ""
		if cfg != nil {
			configuredJava = cfg.RealJavaPath
		}

		logger.Infof("[Lunaris] Resolving real Java runtime path (configured: %q)...", configuredJava)
		detectedJava, err := injector.FindRealJava(configuredJava)
		if err != nil {
			logger.Errorf("[Lunaris] FATAL ERROR: %v", err)
			_ = logger.Sync()
			_ = logger.Close()
			os.Exit(1)
		}
		realJava = detectedJava
	}

	logger.Infof("[Lunaris] Launching Minecraft via: %s", realJava)
	logger.Infof("==================================================")
	logger.Infof("[Lunaris] Pre-launch synchronization completed. Handing over execution to Java runtime.")
	_ = logger.Sync()
	_ = logger.Close()

	exitCode, err := injector.ExecOrRunJava(realJava, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Lunaris] Error running Java: %v\n", err)
	}
	os.Exit(exitCode)
}

func cmdInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	instanceFlag := fs.String("instance", "", "Path to the Minecraft instance directory")
	serverFlag := fs.String("server", config.DefaultServerURL, "Sync server URL (default: https://lunaris.csfrederick.com)")
	versionFlag := fs.String("game-version", "1.20.1", "Required Minecraft version")
	javaFlag := fs.String("java", "", "Path to the real Java binary (auto-detected if empty)")
	profileFile := fs.String("profile-file", "", "Path to launcher_profiles.json (optional)")
	profileID := fs.String("profile-id", "", "Profile ID in launcher_profiles.json (optional)")
	vrFlag := fs.Bool("vr", false, "Enable optional Windows VR mods & configs (Vivecraft)")
	enableVRFlag := fs.Bool("enable-vr", false, "Alias for --vr")
	forceFlag := fs.Bool("force", false, "Force installation and overwrite existing mods without confirmation")
	overwriteFlag := fs.Bool("overwrite-mods", false, "Alias for --force to allow overwriting existing mods")
	cliFlag := fs.Bool("cli", false, "Run in interactive terminal mode instead of graphical installer")
	guiFlag := fs.Bool("gui", false, "Force launch graphical installer")
	_ = fs.Parse(args)

	// Non-interactive mode (when --instance is explicitly passed)
	if *instanceFlag != "" {
		logFiles, _ := logger.Init(*instanceFlag, logger.Options{Version: Version, Args: args})
		defer logger.Close()

		logger.Infof("[Installer] Installing LunarisInjector for: %s", *instanceFlag)
		if len(logFiles) > 0 {
			logger.Infof("[Installer] Action log initialized: %s", strings.Join(logFiles, ", "))
		}

		existingMods := installer.CountExistingMods(*instanceFlag)
		if existingMods > 0 && !config.IsLunarisInstance(*instanceFlag) {
			if !*forceFlag && !*overwriteFlag {
				logger.Warnf("⚠️  WARNING: Existing mods detected!")
				logger.Warnf("   Instance '%s' already contains %d mod file(s) in 'mods/'.", *instanceFlag, existingMods)
				logger.Warnf("   Installing LunarisInjector will synchronize this directory with the remote server,")
				logger.Warnf("   which will OVERWRITE or DELETE local mods not present in the modpack manifest.")
				logger.Warnf("   Pass --force or --overwrite-mods to proceed.")
				os.Exit(1)
			}
			logger.Infof("⚠️  Proceeding with overwrite of %d existing mod(s) in: %s", existingMods, *instanceFlag)
		}

		enableVR := *vrFlag || *enableVRFlag
		err := installer.Install(installer.InstallConfig{
			InstanceDir:         *instanceFlag,
			ServerURL:           *serverFlag,
			RequiredGameVersion: *versionFlag,
			RealJavaPath:        *javaFlag,
			ProfileFile:         *profileFile,
			ProfileID:           *profileID,
			EnableVR:            enableVR,
			Logger:              logger.AsFunc(),
		})
		if err != nil {
			logger.Errorf("❌ Installation failed: %v", err)
			os.Exit(1)
		}
		logger.Infof("✔ LunarisInjector successfully configured for: %s", *instanceFlag)
		logger.Infof("  Sync Server URL: %s", *serverFlag)
		if enableVR {
			logger.Infof("  ✦ Windows VR support (Vivecraft): ENABLED")
		} else {
			logger.Infof("  ✦ Windows VR support (Vivecraft): DISABLED (Standard)")
		}
		runtimes := installer.FindCurseForgeJavaRuntimeDirs(*instanceFlag)
		for _, r := range runtimes {
			if installer.IsCurseForgeJavaHooked(r) {
				logger.Infof("  ✔ CurseForge Java runtime hooked: %s", r)
			}
		}
		logger.Infof("  Ready to play! Simply click 'Play' in CurseForge.")
		return
	}

	// Default to graphical installer unless --cli was explicitly specified
	if !*cliFlag || *guiFlag {
		cmdGUI(args)
		return
	}

	// Interactive CLI mode
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("╭─────────────────────────────────────────────────────────────╮")
	fmt.Println("│                    ✦ LUNARIS INJECTOR ✦                     │")
	fmt.Println("│           Automatic Modpack Synchronizer (1.20.1)           │")
	fmt.Println("╰─────────────────────────────────────────────────────────────╯")
	fmt.Println()
	fmt.Println("Scanning for installed Minecraft instances & profiles...")

	detected := installer.DetectInstances()

	var compatible []installer.InstanceInfo
	var incompatible []installer.InstanceInfo

	for _, inst := range detected {
		if inst.IsCompatible {
			compatible = append(compatible, inst)
		} else {
			incompatible = append(incompatible, inst)
		}
	}

	var selectedPath string
	var selectedName string
	var selectedProfileFile string
	var selectedProfileID string
	enableVR := *vrFlag || *enableVRFlag

	if len(compatible) > 0 {
		if !enableVR && compatible[0].EnableVR {
			enableVR = true
		}
		fmt.Println("\nDetected Minecraft 1.20.1 instance(s):")
		for i, inst := range compatible {
			recTag := ""
			if i == 0 {
				recTag = " ✦ Recommended"
			}
			status := "Ready to connect"
			if inst.IsInjected {
				vrInfo := ""
				if inst.EnableVR {
					vrInfo = ", VR: on"
				}
				status = fmt.Sprintf("Already hooked (%s%s)", inst.ConfiguredServer, vrInfo)
			}
			modNote := ""
			if inst.ModCount > 0 && !inst.IsInjected {
				modNote = fmt.Sprintf(" (⚠️  %d existing mods)", inst.ModCount)
			}
			fmt.Printf("  [%d] %s (%s)%s%s\n", i+1, inst.Name, inst.Launcher, recTag, modNote)
			fmt.Printf("      Path   : %s\n", inst.Path)
			fmt.Printf("      Status : %s\n", status)
		}

		if len(incompatible) > 0 {
			fmt.Println("\nOther instances (incompatible with 1.20.1):")
			for _, inst := range incompatible {
				ver := inst.GameVersion
				if ver == "" {
					ver = "Unknown"
				}
				fmt.Printf("  [-] %s (%s, MC %s) [Requires 1.20.1]\n", inst.Name, inst.Launcher, ver)
			}
		}

		customChoice := len(compatible) + 1
		fmt.Printf("\n  [%d] Specify a custom folder path...\n", customChoice)

		for {
			fmt.Printf("\nSelect an instance [default: 1, or 'q' to quit]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			if input == "" || input == "1" {
				sel := compatible[0]
				selectedPath = sel.Path
				selectedName = sel.Name
				selectedProfileFile = sel.ProfileFile
				selectedProfileID = sel.ProfileID
				if !*vrFlag && !*enableVRFlag {
					enableVR = sel.EnableVR
				}
				break
			}
			if strings.EqualFold(input, "q") {
				fmt.Println("Installation cancelled.")
				return
			}
			idx, err := strconv.Atoi(input)
			if err == nil && idx >= 1 && idx <= len(compatible) {
				sel := compatible[idx-1]
				selectedPath = sel.Path
				selectedName = sel.Name
				selectedProfileFile = sel.ProfileFile
				selectedProfileID = sel.ProfileID
				if !*vrFlag && !*enableVRFlag {
					enableVR = sel.EnableVR
				}
				break
			} else if err == nil && idx == customChoice {
				break
			}
			fmt.Printf("Please enter a number between 1 and %d.\n", customChoice)
		}
	}

	if selectedPath == "" {
		for {
			fmt.Print("\nEnter instance directory path (where 'mods' folder is located): ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" || strings.EqualFold(input, "q") {
				fmt.Println("Installation cancelled.")
				return
			}

			customVer := installer.DetectInstanceVersion(input)
			if customVer != "" && customVer != "1.20.1" {
				fmt.Printf("\n❌ Error: Folder is for Minecraft %s, but this modpack requires 1.20.1.\n", customVer)
				fmt.Println("   Please specify a 1.20.1 instance.")
				continue
			}
			selectedPath = input
			selectedName = filepath.Base(input)
			cfgPath := filepath.Join(input, config.ConfigFileName)
			if cfg, err := config.Load(cfgPath); err == nil && !*vrFlag && !*enableVRFlag {
				enableVR = cfg.EnableVR
			}
			break
		}
	}

	// Auto-filled Server URL
	serverInput := config.DefaultServerURL

	// Auto-detected Java
	detectedJava, _ := injector.FindRealJava("")
	javaInput := detectedJava

	// Display modern Setup Overview card
	renderPlan := func() {
		fmt.Println()
		fmt.Println("╭── Installation Plan ────────────────────────────────────────╮")
		fmt.Printf("│  Instance  : %-46s │\n", truncateStr(selectedName, 46))
		fmt.Printf("│  Location  : %-46s │\n", truncateStr(selectedPath, 46))
		fmt.Printf("│  Server    : %-46s │\n", truncateStr(serverInput, 46))
		javaDisplay := javaInput
		if javaDisplay == "" {
			javaDisplay = "java (system default)"
		}
		fmt.Printf("│  Java 17   : %-46s │\n", truncateStr(javaDisplay, 46))
		fmt.Println("│  Sync Dirs : mods, config, global_packs                     │")
		vrStatus := "Disabled (desktop / non-VR)"
		if enableVR {
			vrStatus = "Enabled (Windows VR / Vivecraft)"
		}
		fmt.Printf("│  Windows VR: %-46s │\n", truncateStr(vrStatus, 46))
		existingMods := installer.CountExistingMods(selectedPath)
		if existingMods > 0 && !config.IsLunarisInstance(selectedPath) {
			warnStr := fmt.Sprintf("⚠️  %d mods will be overwritten!", existingMods)
			fmt.Printf("│  Warning   : %-46s │\n", truncateStr(warnStr, 46))
		}
		fmt.Println("╰─────────────────────────────────────────────────────────────╯")
	}

	renderPlan()

	for {
		fmt.Print("\nReady to install! Press [Enter] to proceed (or 'v' to toggle VR, 'c' to customize, 'q' to quit): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" || strings.EqualFold(input, "y") || strings.EqualFold(input, "yes") {
			break
		} else if strings.EqualFold(input, "v") {
			enableVR = !enableVR
			if enableVR {
				fmt.Println(">> Windows VR mods (Vivecraft) ENABLED.")
			} else {
				fmt.Println(">> Windows VR mods (Vivecraft) DISABLED.")
			}
			renderPlan()
		} else if strings.EqualFold(input, "c") {
			fmt.Printf("\nSync Server URL [default: %s]: ", serverInput)
			srvIn, _ := reader.ReadString('\n')
			srvIn = strings.TrimSpace(srvIn)
			if srvIn != "" {
				serverInput = srvIn
			}

			fmt.Printf("Java Runtime Path [default: %s]: ", javaInput)
			jIn, _ := reader.ReadString('\n')
			jIn = strings.TrimSpace(jIn)
			if jIn != "" {
				javaInput = jIn
			}

			vrDefault := "n"
			if enableVR {
				vrDefault = "y"
			}
			fmt.Printf("Enable Windows VR mods (Vivecraft) [y/n, default: %s]: ", vrDefault)
			vrIn, _ := reader.ReadString('\n')
			vrIn = strings.TrimSpace(vrIn)
			if strings.EqualFold(vrIn, "y") || strings.EqualFold(vrIn, "yes") {
				enableVR = true
			} else if strings.EqualFold(vrIn, "n") || strings.EqualFold(vrIn, "no") {
				enableVR = false
			}

			renderPlan()
		} else if strings.EqualFold(input, "q") {
			fmt.Println("Installation cancelled.")
			return
		}
	}

	// If existing mods are found in an unhooked instance, require explicit confirmation before overwriting
	existingMods := installer.CountExistingMods(selectedPath)
	if existingMods > 0 && !config.IsLunarisInstance(selectedPath) {
		fmt.Println()
		fmt.Println("╭─────────────────────────────────────────────────────────────╮")
		fmt.Println("│ ⚠️  WARNING: EXISTING MODS WILL BE OVERWRITTEN              │")
		fmt.Println("├─────────────────────────────────────────────────────────────┤")
		fmt.Printf("│  This instance already contains %-3d mod file(s).            │\n", existingMods)
		fmt.Println("│  Installing LunarisInjector will synchronize your files with│")
		fmt.Println("│  the server, which will OVERWRITE and DELETE any local mods │")
		fmt.Println("│  that are not present on the server!                        │")
		fmt.Println("╰─────────────────────────────────────────────────────────────╯")
		fmt.Print("\nAre you sure you want to OVERWRITE existing mods? (yes/no): ")
		confirmInput, _ := reader.ReadString('\n')
		confirmInput = strings.TrimSpace(confirmInput)
		if !strings.EqualFold(confirmInput, "yes") && !strings.EqualFold(confirmInput, "y") {
			fmt.Println("Installation cancelled to protect existing mods.")
			return
		}
	}

	selfExe, _ := os.Executable()
	selfExe, _ = filepath.Abs(selfExe)

	logFiles, _ := logger.Init(selectedPath, logger.Options{Version: Version, Args: args})
	defer logger.Close()

	logger.Infof("\nConfiguring LunarisInjector for: %s", selectedPath)
	if len(logFiles) > 0 {
		logger.Infof("[Installer] Action log initialized: %s", strings.Join(logFiles, ", "))
	}

	err := installer.Install(installer.InstallConfig{
		InstanceDir:   selectedPath,
		ServerURL:     serverInput,
		RealJavaPath:  javaInput,
		LunarisBinary: selfExe,
		ProfileFile:   selectedProfileFile,
		ProfileID:     selectedProfileID,
		EnableVR:      enableVR,
		Logger:        logger.AsFunc(),
	})

	if err != nil {
		logger.Errorf("\n❌ Installation error: %v", err)
		return
	}

	// Determine installed wrapper path (copied to instance directory or selfExe)
	wrapperPath := filepath.Join(selectedPath, filepath.Base(selfExe))
	if _, err := os.Stat(wrapperPath); err != nil {
		wrapperPath = selfExe
	}

	// Quick connectivity check
	logger.Infof("Testing server connection (%s)...", serverInput)
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(strings.TrimRight(serverInput, "/") + "/manifest.json")
	if err == nil && resp.StatusCode == http.StatusOK {
		var m manifest.Manifest
		if json.NewDecoder(resp.Body).Decode(&m) == nil {
			logger.Infof("✔ Connected to sync server (%d files ready to sync)", len(m.Files))
		} else {
			logger.Infof("✔ Connected to sync server")
		}
		resp.Body.Close()
	} else {
		logger.Warnf("ℹ Sync server offline or unreachable (%v). Offline launch mode is enabled.", err)
	}

	fmt.Println()
	fmt.Println("╭─────────────────────────────────────────────────────────────╮")
	fmt.Println("│                ✔ INSTALLATION COMPLETE!                     │")
	fmt.Println("╰─────────────────────────────────────────────────────────────╯")
	fmt.Println()
	fmt.Println("LunarisInjector is now seamlessly configured for your modpack!")
	fmt.Printf("  ▸ Modpack Instance : %s\n", selectedName)
	fmt.Printf("  ▸ Instance Folder  : %s\n", selectedPath)
	fmt.Printf("  ▸ Remote Server    : %s\n", serverInput)
	fmt.Println("  ▸ Sync Directories : mods, config, global_packs")
	if enableVR {
		fmt.Println("  ▸ Windows VR       : ENABLED (Vivecraft & VR configs active)")
	} else {
		fmt.Println("  ▸ Windows VR       : DISABLED (Standard desktop play)")
	}

	runtimes := installer.FindCurseForgeJavaRuntimeDirs(selectedPath)
	for _, r := range runtimes {
		if installer.IsCurseForgeJavaHooked(r) {
			fmt.Printf("  ▸ CurseForge Java  : %s (Hooked)\n", filepath.Join(r, "java"))
		}
	}
	fmt.Println()
	fmt.Println("Ready to play:")
	fmt.Printf("  Simply open CurseForge and click 'PLAY' on %s!\n", selectedName)
	fmt.Println("  Lunaris will automatically synchronize your mods, configs, and global packs")
	fmt.Println("  from https://lunaris.csfrederick.com before starting Minecraft.")
	fmt.Println("===============================================================")
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return "..." + s[len(s)-(maxLen-3):]
}

func cmdUninstall(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	instanceFlag := fs.String("instance", "", "Path to the Minecraft instance directory")
	_ = fs.Parse(args)

	target := *instanceFlag
	if target == "" {
		detected := installer.DetectInstances()
		var installed []installer.InstanceInfo
		for _, inst := range detected {
			if inst.IsInjected {
				installed = append(installed, inst)
			}
		}

		reader := bufio.NewReader(os.Stdin)
		if len(installed) > 0 {
			fmt.Println("Configured Lunaris instances:")
			for i, inst := range installed {
				fmt.Printf("  [%d] %s (%s)\n", i+1, inst.Name, inst.Path)
			}
			fmt.Printf("\nSelect an instance to uninstall [default: 1, or enter path]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" || input == "1" {
				target = installed[0].Path
			} else if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(installed) {
				target = installed[idx-1].Path
			} else {
				target = input
			}
		} else {
			fmt.Print("Enter instance directory to uninstall Lunaris from: ")
			input, _ := reader.ReadString('\n')
			target = strings.TrimSpace(input)
		}
	}

	if target == "" {
		fmt.Println("No directory provided.")
		return
	}

	logFiles, _ := logger.Init(target, logger.Options{Version: Version, Args: args})
	defer logger.Close()

	logger.Infof("[Installer] Uninstalling LunarisInjector from: %s", target)
	if len(logFiles) > 0 {
		logger.Infof("[Installer] Action log initialized: %s", strings.Join(logFiles, ", "))
	}

	err := installer.Uninstall(target, "", "", logger.AsFunc())
	if err != nil {
		logger.Errorf("❌ Uninstall error: %v", err)
		return
	}
	logger.Infof("✔ LunarisInjector successfully uninstalled from: %s", target)

	runtimes := installer.FindCurseForgeJavaRuntimeDirs(target)
	for _, r := range runtimes {
		if !installer.IsCurseForgeJavaHooked(r) {
			logger.Infof("✔ Restored stock CurseForge Java runtime at: %s", r)
		}
	}
}

func cmdUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	repoFlag := fs.String("repo", updater.DefaultGitHubRepo, "GitHub repository to check (e.g. user/repo)")
	forceFlag := fs.Bool("force", false, "Force update even if version is up-to-date")
	_ = fs.Parse(args)

	logFiles, _ := logger.Init("", logger.Options{Version: Version, Args: args})
	defer logger.Close()

	if len(logFiles) > 0 {
		logger.Infof("[Updater] Action log initialized: %s", strings.Join(logFiles, ", "))
	}

	updater.CleanupOld()

	logger.Infof("Checking for updates from GitHub (%s)...", *repoFlag)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	info, hasUpdate, err := updater.CheckUpdate(ctx, *repoFlag, Version)
	if err != nil {
		logger.Errorf("❌ Update check failed: %v", err)
		os.Exit(1)
	}

	if !hasUpdate && !*forceFlag {
		logger.Infof("✔ LunarisInjector is up to date (v%s).", Version)
		return
	}

	if info != nil {
		logger.Infof("✦ New version found: v%s (Current: v%s)", info.Version, Version)
		if info.AssetSize > 0 {
			logger.Infof("✦ Asset: %s (%.2f MB)", info.AssetName, float64(info.AssetSize)/(1024*1024))
		}
		if err := updater.SelfUpdate(ctx, info.AssetURL, logger.AsFunc()); err != nil {
			logger.Errorf("❌ Update failed: %v", err)
			os.Exit(1)
		}
		logger.Infof("✔ Successfully updated LunarisInjector to v%s!", info.Version)
	} else if *forceFlag {
		logger.Infof("No newer version found, but --force was specified. No binary replaced.")
	}
}

func cmdServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	dirFlag := fs.String("dir", ".", "Directory to host and sync (e.g. your modpack folder)")
	dirsFlag := fs.String("dirs", "mods,config,global_packs", "Comma-separated list of directories to sync")
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

	var syncDirs []string
	for _, d := range strings.Split(*dirsFlag, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			syncDirs = append(syncDirs, d)
		}
	}

	srv := server.New(server.ServerOptions{
		RootDir:  absDir,
		SyncDirs: syncDirs,
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
	dirsFlag := fs.String("dirs", "mods,config,global_packs", "Comma-separated list of directories to scan")
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

	var syncDirs []string
	for _, d := range strings.Split(*dirsFlag, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			syncDirs = append(syncDirs, d)
		}
	}

	fmt.Printf("Scanning '%s' and generating manifest...\n", absDir)
	m, err := manifest.ScanDirectory(absDir, syncDirs, nil)
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

func cmdResync(args []string) {
	fs := flag.NewFlagSet("resync", flag.ExitOnError)
	instanceFlag := fs.String("instance", "", "Path to the Minecraft instance directory (default: current directory)")
	serverFlag := fs.String("server", "", "Override sync server URL (defaults to lunaris.json config)")
	_ = fs.Parse(args)

	target := *instanceFlag
	if target == "" && fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	if target == "" {
		cwd, _ := os.Getwd()
		target = cwd
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid target path: %v\n", err)
		os.Exit(1)
	}

	logFiles, _ := logger.Init(absTarget, logger.Options{Version: Version, Args: args})
	defer logger.Close()

	logger.Infof("[Resync] Starting modpack resynchronization for: %s", absTarget)
	if len(logFiles) > 0 {
		logger.Infof("[Resync] Action log initialized: %s", strings.Join(logFiles, ", "))
	}

	err = syncer.Resync(context.Background(), syncer.ResyncOptions{
		InstanceDir: absTarget,
		ServerURL:   *serverFlag,
		WorkerCount: 4,
		Logger:      logger.AsFunc(),
	})
	if err != nil {
		logger.Errorf("[Resync] Failed: %v", err)
		os.Exit(1)
	}

	logger.Infof("\n✔ Instance successfully resynced to server modpack!")
	logger.Infof("  All custom modifications have been forgotten and matched to server.")
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

	logFiles, _ := logger.Init(absDir, logger.Options{Version: Version, Args: args})
	defer logger.Close()

	logger.Infof("[Verify] Verifying instance at: %s", absDir)
	if len(logFiles) > 0 {
		logger.Infof("[Verify] Action log initialized: %s", strings.Join(logFiles, ", "))
	}

	cfg, _, err := config.FindInstanceConfig(absDir)
	if err != nil && *serverFlag == "" {
		logger.Errorf("Error: No lunaris.json found and no --server URL specified.")
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
		Logger:      logger.AsFunc(),
	})

	remote, err := s.FetchRemoteManifest(context.Background())
	if err != nil {
		logger.Errorf("Failed to fetch remote manifest: %v", err)
		os.Exit(1)
	}

	plan, err := s.CalculatePlan(remote)
	if err != nil {
		logger.Errorf("Failed to calculate plan: %v", err)
		os.Exit(1)
	}

	logger.Infof("\n=== Verification Report ===")
	if cfg.EnableVR {
		logger.Infof("Feature Mode      : Windows VR Enabled (Vivecraft active)")
	} else {
		logger.Infof("Feature Mode      : Standard Desktop (VR disabled)")
	}
	logger.Infof("Matches / In Sync : %d files", plan.Unchanged)
	logger.Infof("Need Download     : %d files (%.2f MB)", len(plan.Downloads), float64(plan.TotalBytes)/(1024*1024))
	for _, d := range plan.Downloads {
		logger.Infof("  + [ADD/UPDATE] %s (%d bytes)", d.ClientPath(), d.Size)
	}
	logger.Infof("Obsolete / Delete : %d files", len(plan.Deletions))
	for _, del := range plan.Deletions {
		logger.Infof("  - [DELETE]     %s", del)
	}

	if len(plan.Downloads) == 0 && len(plan.Deletions) == 0 {
		logger.Infof("\n✓ Instance is 100%% in sync with the remote server!")
	} else {
		logger.Warnf("\n! Instance has discrepancies compared to the remote server.")
	}
}

func cmdListInstances() {
	fmt.Println("Scanning for installed Minecraft instances...")
	detected := installer.DetectInstances()
	if len(detected) == 0 {
		fmt.Println("No instances automatically detected.")
		return
	}

	fmt.Printf("Found %d instances (Target Modpack Version: 1.20.1):\n\n", len(detected))
	for i, inst := range detected {
		injected := "No"
		if inst.IsInjected {
			vrNote := ""
			if inst.EnableVR {
				vrNote = ", VR enabled"
			}
			injected = fmt.Sprintf("Yes (Syncing with: %s%s)", inst.ConfiguredServer, vrNote)
		}
		verStr := inst.GameVersion
		if verStr == "" {
			verStr = "Unknown"
		}
		compatStr := "❌ Incompatible (Requires 1.20.1)"
		if inst.IsCompatible {
			compatStr = "✓ COMPATIBLE (1.20.1)"
		}

		fmt.Printf("[%d] %s (%s)\n", i+1, inst.Name, inst.Launcher)
		fmt.Printf("    Path        : %s\n", inst.Path)
		fmt.Printf("    Version     : %s [%s]\n", verStr, compatStr)
		fmt.Printf("    Injected    : %s\n", injected)
		if inst.CurrentJava != "" {
			fmt.Printf("    Java        : %s\n", inst.CurrentJava)
		}
		fmt.Println()
	}
}

func cmdGUI(args []string) {
	fs := flag.NewFlagSet("gui", flag.ExitOnError)
	portFlag := fs.Int("port", 0, "Port for GUI server (0 for random available port)")
	noBrowser := fs.Bool("no-browser", false, "Do not auto-open browser window")
	serverURL := fs.String("server", config.DefaultServerURL, "Sync server URL")
	_ = fs.Parse(args)

	srv := gui.New(gui.GUIOptions{
		Port:        *portFlag,
		OpenBrowser: !*noBrowser,
		ServerURL:   *serverURL,
	})

	if err := srv.Start(); err != nil {
		fmt.Printf("GUI error: %v\n", err)
		gui.ShowNativeAlert("Lunaris Installer", fmt.Sprintf("Failed to launch GUI installer:\n%v", err))
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`LunarisInjector v%s - Minecraft Modpack Auto-Synchronizer & Launch Wrapper

USAGE:
  lunaris [command] [arguments]
  Double-click or run with no arguments to launch the graphical installer.

COMMANDS:
  gui           Launch the modern graphical web installer interface
  install       Set up an instance (opens GUI by default; pass --cli for terminal)
  uninstall     Revert an instance back to normal
  resync        Resync instance, forget all custom modifications, and match server
  update        Check GitHub and update LunarisInjector to the latest release
  server        Run a built-in sync server with live manifest & file downloads
  generate      Generate a static manifest.json from a folder (for Nginx, S3, etc.)
  verify        Check an instance's files against the server without launching
  instances     List all detected CurseForge, Vanilla, and Prism instances
  run           Manually test the injector wrapper on a game directory
  version       Print version information

DEFAULT SERVER:
  https://lunaris.csfrederick.com

EXAMPLES:
  # 1. Launch the graphical installer (same as double-clicking the executable)
  lunaris
  # or
  lunaris gui

  # 2. Run terminal interactive installer
  lunaris install --cli

  # 3. Non-interactive install with optional Windows VR (Vivecraft) enabled
  lunaris install --instance ~/.minecraft --vr

  # 4. Host a sync server for players from your modpack folder
  lunaris server --dir ./my-modpack --port 8080

  # 5. Check instance sync status
  lunaris verify --instance ~/.minecraft

  # 6. Resync instance and wipe all custom modifications
  lunaris resync --instance ~/.minecraft

`, Version)
}
