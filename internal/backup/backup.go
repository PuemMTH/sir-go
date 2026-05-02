package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/spf13/cobra"

	"sir/internal/styles"
)

type R2Config struct {
	AccountID       string `json:"account_id"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	BucketName      string `json:"bucket_name"`
}

type BackupSettings struct {
	R2 R2Config `json:"r2"`
}

func settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sir", "settings.json"), nil
}

func loadSettings() (BackupSettings, error) {
	p, err := settingsPath()
	if err != nil {
		return BackupSettings{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return BackupSettings{}, err
	}
	var s BackupSettings
	return s, json.Unmarshal(data, &s)
}

func saveSettings(s BackupSettings) error {
	p, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func newR2Client(r2 R2Config) *s3.Client {
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", r2.AccountID)
	awsCfg := aws.Config{
		Region:      "auto",
		Credentials: aws.NewCredentialsCache(awscreds.NewStaticCredentialsProvider(r2.AccessKeyID, r2.SecretAccessKey, "")),
	}
	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
}

func pgDumpContainer(ctx context.Context, cli *client.Client, containerID, pgUser, dbName string) ([]byte, error) {
	execResp, err := cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          []string{"pg_dump", "-U", pgUser, dbName},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	attach, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}
	defer attach.Close()

	var stdout, stderr bytes.Buffer
	if _, err = stdcopy.StdCopy(&stdout, &stderr, attach.Reader); err != nil {
		return nil, fmt.Errorf("read output: %w", err)
	}

	insp, err := cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return nil, fmt.Errorf("exec inspect: %w", err)
	}
	if insp.ExitCode != 0 {
		if insp.ExitCode == 127 {
			return nil, fmt.Errorf("pg_dump not found in container — is this a PostgreSQL container?")
		}
		return nil, fmt.Errorf("pg_dump exited %d: %s", insp.ExitCode, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func runBackup(containerID, pgUser, dbName string) error {
	s, err := loadSettings()
	if err != nil {
		return fmt.Errorf("no settings — run 'sir autobackup config set': %w", err)
	}
	if s.R2.AccountID == "" {
		return fmt.Errorf("R2 not configured — run 'sir autobackup config set'")
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}
	defer cli.Close()

	styles.CCyan.Printf("  → Dumping '%s' from container %s...\n", dbName, containerID)
	sqlData, err := pgDumpContainer(ctx, cli, containerID, pgUser, dbName)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err = gz.Write(sqlData); err != nil {
		return err
	}
	_ = gz.Close()

	compressed := buf.Bytes()
	ts := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	key := fmt.Sprintf("backups/%s/%s-%s.sql.gz", dbName, dbName, ts)

	styles.CCyan.Printf("  → Uploading to R2 '%s/%s'...\n", s.R2.BucketName, key)
	r2 := newR2Client(s.R2)
	_, err = r2.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.R2.BucketName),
		Key:           aws.String(key),
		Body:          bytes.NewReader(compressed),
		ContentType:   aws.String("application/gzip"),
		ContentLength: aws.Int64(int64(len(compressed))),
	})
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	styles.CCyan.Printf("  ✓ Backup uploaded: %s/%s (%d KB)\n", s.R2.BucketName, key, len(compressed)/1024)
	return nil
}

const cronMarker = "# sir-autobackup"

func setCronJob(schedule, containerID, pgUser, dbName string) error {
	selfPath, err := os.Executable()
	if err != nil {
		return err
	}
	entry := fmt.Sprintf(`%s %s autobackup run --container %s --user %s --db %s %s`,
		schedule, selfPath, containerID, pgUser, dbName, cronMarker)

	existing, _ := exec.Command("crontab", "-l").Output()
	var kept []string
	for _, line := range strings.Split(string(existing), "\n") {
		if line != "" && !strings.Contains(line, cronMarker) {
			kept = append(kept, line)
		}
	}
	kept = append(kept, entry)

	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(strings.Join(kept, "\n") + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("crontab: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func removeCronJob() (bool, error) {
	existing, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		return false, nil
	}
	var kept []string
	removed := false
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.Contains(line, cronMarker) {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if !removed {
		return false, nil
	}
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(strings.Join(kept, "\n"))
	return true, cmd.Run()
}

func NewCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "autobackup",
		Short: "Backup PostgreSQL from a Docker container to Cloudflare R2",
	}

	cfgCmd := &cobra.Command{Use: "config", Short: "Manage R2 credentials"}

	var (
		flagAccountID  string
		flagAccessKey  string
		flagSecretKey  string
		flagBucketName string
	)
	cfgSetCmd := &cobra.Command{
		Use:   "set",
		Short: "Save R2 credentials to ~/.sir/settings.json",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			s := BackupSettings{R2: R2Config{
				AccountID:       flagAccountID,
				AccessKeyID:     flagAccessKey,
				SecretAccessKey: flagSecretKey,
				BucketName:      flagBucketName,
			}}
			if err := saveSettings(s); err != nil {
				styles.CRed.Printf("  Error: %v\n", err)
				os.Exit(1)
			}
			p, _ := settingsPath()
			styles.CCyan.Printf("  ✓ Settings saved to %s\n", p)
		},
	}
	cfgSetCmd.Flags().StringVar(&flagAccountID, "account-id", "", "Cloudflare account ID")
	cfgSetCmd.Flags().StringVar(&flagAccessKey, "access-key", "", "R2 access key ID")
	cfgSetCmd.Flags().StringVar(&flagSecretKey, "secret-key", "", "R2 secret access key")
	cfgSetCmd.Flags().StringVar(&flagBucketName, "bucket", "", "R2 bucket name")
	_ = cfgSetCmd.MarkFlagRequired("account-id")
	_ = cfgSetCmd.MarkFlagRequired("access-key")
	_ = cfgSetCmd.MarkFlagRequired("secret-key")
	_ = cfgSetCmd.MarkFlagRequired("bucket")

	cfgShowCmd := &cobra.Command{
		Use:   "show",
		Short: "Print current R2 configuration",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			s, err := loadSettings()
			if err != nil {
				styles.CRed.Printf("  No config — run 'sir autobackup config set'\n")
				return
			}
			mask := func(v string) string {
				if len(v) <= 4 {
					return "****"
				}
				return v[:4] + strings.Repeat("*", len(v)-4)
			}
			styles.CCyan.Printf("  account_id:        %s\n", s.R2.AccountID)
			styles.CCyan.Printf("  access_key_id:     %s\n", s.R2.AccessKeyID)
			styles.CCyan.Printf("  secret_access_key: %s\n", mask(s.R2.SecretAccessKey))
			styles.CCyan.Printf("  bucket_name:       %s\n", s.R2.BucketName)
		},
	}
	cfgCmd.AddCommand(cfgSetCmd, cfgShowCmd)

	var (
		runContainer string
		runUser      string
		runDB        string
	)
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run a backup immediately",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := runBackup(runContainer, runUser, runDB); err != nil {
				styles.CRed.Printf("  Error: %v\n", err)
				os.Exit(1)
			}
		},
	}
	runCmd.Flags().StringVar(&runContainer, "container", "", "container name or ID")
	runCmd.Flags().StringVar(&runUser, "user", "postgres", "PostgreSQL user")
	runCmd.Flags().StringVar(&runDB, "db", "", "database name")
	_ = runCmd.MarkFlagRequired("container")
	_ = runCmd.MarkFlagRequired("db")

	cronCmd := &cobra.Command{Use: "cron", Short: "Manage scheduled backups"}

	var (
		cronSchedule  string
		cronContainer string
		cronUser      string
		cronDB        string
	)
	cronSetCmd := &cobra.Command{
		Use:   "set",
		Short: "Install a cron job for automatic backups",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := setCronJob(cronSchedule, cronContainer, cronUser, cronDB); err != nil {
				styles.CRed.Printf("  Error: %v\n", err)
				os.Exit(1)
			}
			styles.CCyan.Printf("  ✓ Cron job set (%s)\n", cronSchedule)
		},
	}
	cronSetCmd.Flags().StringVar(&cronSchedule, "schedule", "", "cron expression, e.g. '0 2 * * *'")
	cronSetCmd.Flags().StringVar(&cronContainer, "container", "", "container name or ID")
	cronSetCmd.Flags().StringVar(&cronUser, "user", "postgres", "PostgreSQL user")
	cronSetCmd.Flags().StringVar(&cronDB, "db", "", "database name")
	_ = cronSetCmd.MarkFlagRequired("schedule")
	_ = cronSetCmd.MarkFlagRequired("container")
	_ = cronSetCmd.MarkFlagRequired("db")

	cronRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove the sir autobackup cron job",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			removed, err := removeCronJob()
			if err != nil {
				styles.CRed.Printf("  Error: %v\n", err)
				os.Exit(1)
			}
			if removed {
				styles.CCyan.Printf("  ✓ Cron job removed\n")
			} else {
				styles.CYellow.Printf("  No sir autobackup cron job found\n")
			}
		},
	}

	cronStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current autobackup cron job",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			out, err := exec.Command("crontab", "-l").Output()
			if err != nil {
				styles.CRed.Printf("  No crontab found\n")
				return
			}
			found := false
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, cronMarker) {
					styles.CCyan.Printf("  %s\n", line)
					found = true
				}
			}
			if !found {
				styles.CYellow.Printf("  No sir autobackup cron job found\n")
			}
		},
	}

	cronCmd.AddCommand(cronSetCmd, cronRemoveCmd, cronStatusCmd)
	root.AddCommand(cfgCmd, runCmd, cronCmd, newTUICmd())
	return root
}
