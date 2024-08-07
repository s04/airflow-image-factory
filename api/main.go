package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

var (
	REGISTRY_URL = os.Getenv("REGISTRY_URL") // set in .env file... It's being .gitignored
	IMAGE_NAME   = os.Getenv("IMAGE_NAME")   // set in .env file... It's being .gitignored
)

func init() {
	if REGISTRY_URL == "" {
		REGISTRY_URL = "localhost:5000" // default value
	}
	if IMAGE_NAME == "" {
		IMAGE_NAME = "airflow" // default value
	}
	fmt.Printf("Using Registry URL: %s\n", REGISTRY_URL)
	fmt.Printf("Using Image Name: %s\n", IMAGE_NAME)
}

type DockerBuildRequest struct {
	AirflowVersion string   `json:"airflow_version"`
	PythonVersion  string   `json:"python_version"`
	BaseImage      string   `json:"base_image"`
	Extras         []string `json:"extras"`
	AptDeps        []string `json:"apt_deps"`
	PipDeps        []string `json:"pip_deps"`
}

const dockerfileTemplate = `
FROM apache/airflow:{{.AirflowVersion}}-python{{.PythonVersion}}

USER root

# Install apt dependencies
RUN apt-get update && apt-get install -y --no-install-recommends {{range .AptDeps}}{{.}} {{end}} && \
    apt-get autoremove -yqq --purge && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

USER airflow

# Install Airflow with extras and additional pip dependencies
RUN pip install --no-cache-dir "apache-airflow[{{StringsJoin .Extras ","}}]=={{.AirflowVersion}}" {{range .PipDeps}}{{.}} {{end}}

CMD ["airflow"]
`

func generateTag(req DockerBuildRequest) string {
	data, _ := json.Marshal(req)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8]) // Use first 8 bytes of hash
}

func buildAndPushDocker(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received build and push request")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DockerBuildRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Printf("Received request: %+v\n", req)

	tmpl, err := template.New("dockerfile").Funcs(template.FuncMap{
		"StringsJoin": strings.Join,
	}).Parse(dockerfileTemplate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var dockerfile bytes.Buffer
	err = tmpl.Execute(&dockerfile, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Println("Generated Dockerfile:")
	fmt.Println(dockerfile.String())

	// Write Dockerfile
	err = os.WriteFile("Dockerfile", dockerfile.Bytes(), 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate tag from request parameters
	tag := generateTag(req)
	fmt.Printf("Generated tag: %s\n", tag)

	// Build Docker image
	imageName := fmt.Sprintf("%s/%s:%s", REGISTRY_URL, IMAGE_NAME, tag)
	buildCmd := exec.Command("docker", "build", "-t", imageName, ".")
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		errMsg := fmt.Sprintf("Docker build failed: %s\n%s", err, buildOutput)
		fmt.Println(errMsg)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	fmt.Printf("Docker build output:\n%s\n", buildOutput)

	// Push Docker image
	pushCmd := exec.Command("docker", "push", imageName)
	pushOutput, err := pushCmd.CombinedOutput()
	if err != nil {
		errMsg := fmt.Sprintf("Docker push failed: %s\n%s", err, pushOutput)
		fmt.Println(errMsg)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	fmt.Printf("Docker push output:\n%s\n", pushOutput)

	w.WriteHeader(http.StatusOK)
	responseMsg := fmt.Sprintf("Docker image built and pushed successfully: %s", imageName)
	fmt.Println(responseMsg)
	io.WriteString(w, responseMsg+"\n")
}

func main() {
	http.HandleFunc("/build-and-push", buildAndPushDocker)
	fmt.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}