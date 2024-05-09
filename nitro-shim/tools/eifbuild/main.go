package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/google/shlex"
)

const (
	bootstrapTemplate = `files:
  - path: dev
    directory: true
    mode: "0755"
  - path: init
    source: {{ .initPath }}
    mode: "0755"
  - path: nsm.ko
    source: {{ .nsmkoPath }}
    mode: "0755"`
	customerTemplate = `init:
  - {{ .image }}
files:
  - path: rootfs/dev
    directory: true
    mode: "0755"
  - path: rootfs/run
    directory: true
    mode: "0755"
  - path: rootfs/sys
    directory: true
    mode: "0755"
  - path: rootfs/var
    directory: true
    mode: "0755"
  - path: rootfs/proc
    directory: true
    mode: "0755"
  - path: rootfs/tmp
    directory: true
    mode: "0755"
  - path: cmd
    source: {{ .cmd }}
    mode: "0644"
  - path: env
    source: {{ .env }}
    mode: "0644"`
)

type OciIndex struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Manifests     []struct {
		MediaType   string `json:"mediaType"`
		Size        int    `json:"size"`
		Digest      string `json:"digest"`
		Annotations struct {
			OrgOpencontainersImageRefName string `json:"org.opencontainers.image.ref.name"`
		} `json:"annotations"`
	} `json:"manifests"`
}

func printusage() {
	fmt.Println("Usage:\n")
	fmt.Println("eifbuild [-blobs-path BLOBS_PATH] -pass-envs ENVS [-docker-uri IMAGE] -output-file OUTPUT -- COMMAND...\n")

	flag.PrintDefaults()
}

func printhelp() {
	fmt.Println("eifbuild is a tool for building enclave image files.\n")
	printusage()
}

func main() {
	var help bool

	passEnvPtr := flag.String("pass-env", "", "Comma separated list of env vars to pass to the build")
	imagePtr := flag.String("docker-uri", "", "Docker image URI")
	outPtr := flag.String("output-file", "", "Output file for built EIF")
	blobsPath := flag.String("blobs-path", "/usr/share/nitro_enclaves/blobs/", "Path to aws nitro cli blobs")
	flag.BoolVar(&help, "help", false, "Show help")
	flag.BoolVar(&help, "h", false, "Show help (shorthand)")
	flag.Usage = printusage
	flag.Parse()

	if help {
		printhelp()
		os.Exit(0)
	}

	if *outPtr == "" {
		fmt.Println("Error: -output-file flag must be set!")
		printusage()
		os.Exit(1)
	}

	if *imagePtr == "" {
		fmt.Println("No -docker-uri was passed, defaulting to use oci layout path from last kaniko build\n")
		imageName, err := PrepareOciIndex()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		*imagePtr = imageName
	}

	fmt.Println("Image:", *imagePtr, "\n")
	fmt.Println("Output:", *outPtr, "\n")

	cmd := make([]string, 0)
	afterSep := false
	for _, arg := range os.Args {
		if afterSep {
			cmd = append(cmd, arg)
		}
		if arg == "--" {
			afterSep = true
		}
	}

	if len(cmd) == 1 {
		var err error
		// try to split according to shell style tokenization
		cmd, err = shlex.Split(cmd[0])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	fmt.Println("Command:", cmd, "\n")
	fmt.Println("Env:")

	envs := make(map[string]string)
	if *passEnvPtr != "" {
		passEnv := strings.Split(*passEnvPtr, ",")
		for _, k := range passEnv {
			v, ok := os.LookupEnv(k)
			if !ok {
				fmt.Println("Warning:", k, "not present in environment but requested to be passed")
				continue
			}
			envs[k] = v
			fmt.Println(k, "=", v)
		}
	}

	fmt.Println("\nBuilding...")

	err := BuildEif(*blobsPath, *imagePtr, cmd, envs, *outPtr)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	os.Exit(0)
}

func generateBootstrap(initPath, nsmkoPath string) (*os.File, error) {
	file, err := os.CreateTemp("", "bootstrap")
	if err != nil {
		return nil, err
	}
	templ := template.Must(template.New("bootstrap").Parse(bootstrapTemplate))
	err = templ.Execute(file, map[string]interface{}{
		"initPath":  initPath,
		"nsmkoPath": nsmkoPath,
	})
	return file, err
}

func generateCustomer(image, cmdPath, envPath string) (*os.File, error) {
	file, err := os.CreateTemp("", "customer")
	if err != nil {
		return nil, err
	}
	templ := template.Must(template.New("customer").Parse(customerTemplate))
	err = templ.Execute(file, map[string]interface{}{
		"image": image,
		"cmd":   cmdPath,
		"env":   envPath,
	})
	return file, err
}

func PrepareOciIndex() (imageName string, err error) {
	homePath, err := os.UserHomeDir()
	if err != nil {
		return imageName, err
	}
	cachePath := filepath.Join(homePath, ".linuxkit/cache")
	indexPath := filepath.Join(cachePath, "index.json")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return imageName, err
	}

	index := OciIndex{}
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return imageName, err
	}

	if len(index.Manifests) != 1 {
		return imageName, fmt.Errorf("Error: Length of manifests must be 1!")
	}

	manifest := index.Manifests[0]
	digest := manifest.Digest
	imageName = "docker.io/library/" + digest
	currentRef := manifest.Annotations.OrgOpencontainersImageRefName
	if len(currentRef) > 0 && currentRef != imageName {
		return imageName, fmt.Errorf("Error: Image with unexpected name!")
	}
	index.Manifests[0].Annotations.OrgOpencontainersImageRefName = imageName

	indexBytes, err = json.Marshal(&index)
	if err != nil {
		return imageName, err
	}

	err = os.WriteFile(indexPath, indexBytes, 755)
	if err != nil {
		return imageName, err
	}

	return imageName, nil
}

func BuildEif(blobsPath string, image string, cmds []string, envs map[string]string, output string) error {
	artifactsDir, err := os.MkdirTemp("", "initramfs")
	if err != nil {
		return err
	}
	defer os.RemoveAll(artifactsDir)

	bootstrap, err := generateBootstrap(filepath.Join(blobsPath, "init"), filepath.Join(blobsPath, "nsm.ko"))
	if err != nil {
		return err
	}
	defer os.Remove(bootstrap.Name())

	cmd, err := os.CreateTemp("", "cmd")
	if err != nil {
		return err
	}
	defer os.Remove(cmd.Name())

	env, err := os.CreateTemp("", "env")
	if err != nil {
		return err
	}
	defer os.Remove(env.Name())

	// TODO for now we will ignore the cmd and env from the docker image
	for _, c := range cmds {
		fmt.Fprintf(cmd, "%s\n", c)
	}

	// sort the env vars to ensure they are reproducible
	keys := make([]string, len(envs))
	i := 0
	for k := range envs {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := envs[k]
		fmt.Fprintf(env, "%s=%s\n", k, v)
	}

	customer, err := generateCustomer(image, cmd.Name(), env.Name())
	if err != nil {
		return err
	}
	defer os.Remove(customer.Name())

	bootstrapRamdisk := filepath.Join(artifactsDir, "bootstrap-initrd.img")
	customerRamdisk := filepath.Join(artifactsDir, "customer-initrd.img")

	command := execCommand(filepath.Join(blobsPath, "linuxkit"),
		"build",
		"-name",
		filepath.Join(artifactsDir, "bootstrap"),
		"-format",
		"kernel+initrd",
		bootstrap.Name(),
	)
	if err = command.Run(); err != nil {
		return err
	}

	command = execCommand(filepath.Join(blobsPath, "linuxkit"),
		"build",
		"-name",
		filepath.Join(artifactsDir, "customer"),
		"-format",
		"kernel+initrd",
		"-prefix",
		"rootfs/",
		customer.Name(),
	)
	if err = command.Run(); err != nil {
		return err
	}

	cmdline, err := os.ReadFile(filepath.Join(blobsPath, "cmdline"))
	if err != nil {
		return err
	}
	command = execCommand("eif_build",
		"--kernel",
		filepath.Join(blobsPath, "bzImage"),
		"--kernel_config",
		filepath.Join(blobsPath, "bzImage.config"),
		"--cmdline",
		string(cmdline),
		"--ramdisk",
		bootstrapRamdisk,
		"--ramdisk",
		customerRamdisk,
		"--output",
		output,
	)
	if err = command.Run(); err != nil {
		return err
	}
	return nil
}

func execCommand(name string, arg ...string) *exec.Cmd {
	fmt.Println("Running:", name, arg)

	command := exec.Command(name, arg...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command
}
