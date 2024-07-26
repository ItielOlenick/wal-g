package functests

import (
	"fmt"
	"math"
	"syscall"
	"time"

	"github.com/cucumber/godog"
	"github.com/pkg/errors"
	"github.com/wal-g/tracelog"
	"github.com/wal-g/wal-g/tests_func/helpers"
	"github.com/wal-g/wal-g/tests_func/utils"
)

const (
	tmpFile = "/tmp/data/t.tmp"
)

func SetupRedisSteps(ctx *godog.ScenarioContext, tctx *TestContext) {
	ctx.Step(`^redis stopped on ([^\s]*)$`, tctx.redisStoppedOn)
	ctx.Step(`^a working redis on ([^\s]*)$`, tctx.isWorkingRedis)
	ctx.Step(`^([^\s]*) has test redis data test(\d+)$`, tctx.redisHasTestRedisDataTest)
	ctx.Step(`^we create ([^\s]*) ([^\s]*)-redis-backup with ([^\s]*)$`, tctx.createRedisBackup)
	ctx.Step(`^we delete redis backups retain (\d+) via ([^\s]*)$`, tctx.weDeleteRedisBackupsRetainViaRedis)
	ctx.Step(`^we restart redis-server at ([^\s]*)$`, tctx.weRestartRedisServerAt)
	ctx.Step(`^we stop redis-server at ([^\s]*)$`, tctx.weStopRedisServerAt)
	ctx.Step(`^we start redis-server at ([^\s]*)$`, tctx.weStartRedisServerAt)
	ctx.Step(`^we got same redis data at ([^\s]*) ([^\s]*)$`, tctx.testEqualRedisDataAtHosts)
	ctx.Step(`^([^\s]*) manifest is not empty$`, tctx.manifestIsNotEmpty)
	ctx.Step(`^([^\s]*) has heavy write$`, tctx.hasHeavyWrite)
	ctx.Step(`^we stop heavy write on ([^\s]*)$`, tctx.weStopHeavyWriteOn)
	ctx.Step(`^we fill disk on ([^\s]*)$`, tctx.weFillDiskOn)
	ctx.Step(`^we clean disk on ([^\s]*)$`, tctx.weCleanDiskOn)
	ctx.Step(`^we restore #(\d+) aof ([^\s]*) version backup to ([^\s]*)$`, tctx.weRestoreAofBackupToRedis)
}

func (tctx *TestContext) weRestoreAofBackupToRedis(backupNum int, matchVersion string, container string) error {
	var version string
	if matchVersion == "same" {
		version = tctx.Version.Full
	} else if matchVersion == "wrong" {
		version = "5.50.50"
	} else {
		return fmt.Errorf("known options for matchVersion are same and wrong")
	}
	walg := WalgUtilFromTestContext(tctx, container)
	err := walg.FetchAofBackupByNum(backupNum, version)
	if matchVersion == "same" && err != nil {
		return err
	}
	if matchVersion == "wrong" && err == nil {
		return fmt.Errorf("expected error on wrong version")
	}
	return nil
}

func getFilledGigabytes() (int, error) {
	fs := syscall.Statfs_t{}
	err := syscall.Statfs("/tmp", &fs)
	if err != nil {
		return 0, err
	}

	all := fs.Blocks * uint64(fs.Bsize)
	free := fs.Bfree * uint64(fs.Bsize)
	used := all - free

	curUsed := float64(used) / float64(all)
	neccessaryUsed := float64(used) * 0.91 / curUsed
	resInGigabytes := int(neccessaryUsed / math.Pow(2, 30))
	return resInGigabytes, nil
}

func (tctx *TestContext) weFillDiskOn(hostName string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, hostName)
	if err != nil {
		return err
	}
	host := rc.Host()

	filledGigabytes, err := getFilledGigabytes()
	if err != nil {
		return err
	}

	cmd := []string{"fallocate", "-l", fmt.Sprintf("%dG", filledGigabytes), tmpFile}
	_, err = helpers.RunCommandStrict(tctx.Context, host, cmd)
	if err != nil {
		return fmt.Errorf("error creating large file: %+v", err)
	}
	return nil
}

func (tctx *TestContext) weCleanDiskOn(hostName string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, hostName)
	if err != nil {
		return err
	}
	host := rc.Host()

	cmd := []string{"rm", "-rf", tmpFile}
	_, err = helpers.RunCommandStrict(tctx.Context, host, cmd)
	if err != nil {
		return fmt.Errorf("removing large file: %+v", err)
	}
	return nil
}

func (tctx *TestContext) hasHeavyWrite(hostName string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, hostName)
	if err != nil {
		return err
	}
	host := rc.Host()

	cmd := "redis-benchmark -a password -t set -n 100000 -d 100000"
	err = helpers.RunAsyncCommand(tctx.Context, host, cmd)
	if err != nil {
		return fmt.Errorf("heavy write cmd err: %+v", err)
	}

	return nil
}

func (tctx *TestContext) weStopHeavyWriteOn(hostName string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, hostName)
	if err != nil {
		return err
	}
	host := rc.Host()

	cmd := []string{"pkill", "redis-benchmark"}
	_, err = helpers.RunCommandStrict(tctx.Context, host, cmd)
	if err != nil {
		return fmt.Errorf("heavy write stop err: %+v", err)
	}

	return nil
}

func (tctx *TestContext) manifestIsNotEmpty(hostName string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, hostName)
	if err != nil {
		return err
	}
	host := rc.Host()

	return helpers.Retry(tctx.Context, MAX_RETRIES_COUNT, func() error {
		cmd := []string{"stat", "--printf=\"%s\"", "/tmp/data/appendonlydir/appendonly.aof.manifest"}
		res, err := helpers.RunCommandStrict(tctx.Context, host, cmd)
		if err != nil {
			return fmt.Errorf("manifest is missing")
		}
		if res.Stdout() == "0" {
			return fmt.Errorf("manifest is empty")
		}

		return nil
	})
}

func (tctx *TestContext) redisStoppedOn(hostName string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, hostName)
	if err != nil {
		return err
	}
	host := rc.Host()

	cmd := []string{"bash", "-c", "ps aux | grep [r]edis-server"}
	res, err := helpers.RunCommandStrict(tctx.Context, host, cmd)
	if err != nil && res.ExitCode == 1 && res.Stdout() == "" && res.Stderr() == "" {
		return nil
	}

	return fmt.Errorf("unexpected result of checking running redis: %+v, %d: %s: %s", err, res.ExitCode, res.Stdout(), res.Stderr())
}

func (tctx *TestContext) isWorkingRedis(hostName string) error {
	redisCtl, err := GetRedisCtlFromTestContext(tctx, hostName)
	if err != nil {
		return err
	}

	return helpers.Retry(tctx.Context, MAX_RETRIES_COUNT, func() error {
		tracelog.DebugLogger.Printf("Redis client connect to host '%s'", redisCtl.Addr())

		status := redisCtl.Ping()
		err = status.Err()
		if err != nil {
			return fmt.Errorf("Client on ping returned err: %v\n", err)
		}
		if status.Val() != "PONG" {
			return fmt.Errorf("Client on ping does not returned PONG: %v\n", err)
		}
		tracelog.DebugLogger.Printf("Redis: Got PONG on PING from %s", hostName)
		return nil
	})
}

func (tctx *TestContext) redisHasTestRedisDataTest(host string, testId int) error {
	rc, err := GetRedisCtlFromTestContext(tctx, host)
	if err != nil {
		return err
	}

	docsCount := 10
	err = rc.WriteTestData(fmt.Sprintf("test%02d", testId), docsCount)
	if err != nil {
		return err
	}
	return nil
}

func (tctx *TestContext) createRedisBackup(host, backupType, resultType string) error {
	if resultType != "success" && resultType != "error" {
		return fmt.Errorf("undefined resultType: use success or error")
	}

	rc, err := GetRedisCtlFromTestContext(tctx, host)
	if err != nil {
		return nil
	}

	beforeBackupTime, err := helpers.TimeInContainer(tctx.Context, rc.Host())
	if err != nil {
		return err
	}

	passed := beforeBackupTime.Sub(tctx.PreviousBackupTime)
	if passed < time.Second {
		cmd := []string{"sleep", "1"}
		if _, err := helpers.RunCommandStrict(tctx.Context, host, cmd); err != nil {
			return err
		}
	}

	tracelog.DebugLogger.Printf("Push redis %s backup\n", backupType)
	backupId, err := rc.PushBackup(backupType)
	if err != nil && resultType == "success" {
		return err
	}
	if err == nil && resultType == "error" {
		return fmt.Errorf("no expected error occurred")
	}

	time.Sleep(1 * time.Second)
	tracelog.DebugLogger.Println("Backup created: ", backupId)
	return nil
}

func (tctx *TestContext) weDeleteRedisBackupsRetainViaRedis(backupsRetain int, host string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, host)
	if err != nil {
		return err
	}

	return rc.PurgeRetain(backupsRetain)
}

func (tctx *TestContext) weRestartRedisServerAt(host string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, host)
	if err != nil {
		return err
	}
	cmd := rc.ShutdownNoSave()
	if cmd.Err() != nil {
		return cmd.Err()
	}
	return nil
}

func (tctx *TestContext) weStopRedisServerAt(hostName string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, hostName)
	if err != nil {
		return err
	}
	host := rc.Host()

	cmd := []string{"supervisorctl", "stop", "redis"}
	_, err = helpers.RunCommandStrict(tctx.Context, host, cmd)
	if err != nil {
		return fmt.Errorf("stop redis failed: %+v", err)
	}

	return nil
}

func (tctx *TestContext) weStartRedisServerAt(hostName string) error {
	rc, err := GetRedisCtlFromTestContext(tctx, hostName)
	if err != nil {
		return err
	}
	host := rc.Host()

	cmd := []string{"supervisorctl", "start", "redis"}
	_, err = helpers.RunCommandStrict(tctx.Context, host, cmd)
	if err != nil {
		return fmt.Errorf("start redis failed: %+v", err)
	}

	return nil
}

func (tctx *TestContext) testEqualRedisDataAtHosts(host1, host2 string) error {
	rc1, err := GetRedisCtlFromTestContext(tctx, host1)
	if err != nil {
		return err
	}

	rc2, err := GetRedisCtlFromTestContext(tctx, host2)
	if err != nil {
		return err
	}

	dbsize1 := rc1.DBSize()
	if dbsize1.Err() != nil {
		return errors.Wrapf(dbsize1.Err(), "Host %s doesn't return 'dbsize'", host1)
	}
	dbsize2 := rc2.DBSize()
	if dbsize2.Err() != nil {
		return errors.Wrapf(dbsize1.Err(), "Host %s doesn't return 'dbsize'", host2)
	}
	if dbsize1.Val() != dbsize2.Val() {
		return fmt.Errorf("hosts %s and %s have not equal keys count %d != %d", host1, host2, dbsize1.Val(), dbsize2.Val())
	}

	keys1 := rc1.Keys("*")
	if keys1.Err() != nil {
		return keys1.Err()
	}

	keys2 := rc2.Keys("*")
	if keys2.Err() != nil {
		return keys2.Err()
	}

	if len(keys1.Val()) == 0 || len(keys2.Val()) == 0 {
		return fmt.Errorf("keys1 or keys2 is empty - broken backup")
	}

	if !utils.IsArraysEqual(keys1.Val(), keys2.Val()) {
		return fmt.Errorf("keys from redis1/redis2 aren't equal")
	}
	values1 := rc1.MGet(keys1.Val()...)
	values2 := rc1.MGet(keys2.Val()...)
	vals1 := make([]string, len(values1.Val()))
	vals2 := make([]string, len(values1.Val()))

	for i, val := range values1.Val() {
		vals1[i] = val.(string)
	}

	for i, val := range values2.Val() {
		vals2[i] = val.(string)
	}
	if !utils.IsArraysEqual(vals1, vals2) {
		return fmt.Errorf("values from redis1/redis2 aren't equal")
	}
	return nil
}
