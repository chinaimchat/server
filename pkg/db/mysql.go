package db

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // mysql
	"github.com/gocraft/dbr/v2"
	migrate "github.com/rubenv/sql-migrate"
)

// NewMySQL 创建一个MySQL db，[path]db存储路径 [sqlDir]sql脚本目录
func NewMySQL(addr string, sqlDir string, migration bool) *dbr.Session {
	// 强制在每个新建的连接上执行 SET NAMES，避免连接池导致的字符集设置不一致。
	// go-sql-driver/mysql 支持 initCommand DSN 参数。
	if !strings.Contains(addr, "initCommand=") && !strings.Contains(addr, "init_command=") {
		if strings.Contains(addr, "?") {
			addr = addr + "&initCommand=SET%20NAMES%20utf8mb4"
		} else {
			addr = addr + "?initCommand=SET%20NAMES%20utf8mb4"
		}
	}

	conn, err := dbr.Open("mysql", addr, nil)
	if err != nil {
		panic(err)
	}
	conn.SetMaxOpenConns(2000)
	conn.SetMaxIdleConns(1000)
	conn.SetConnMaxLifetime(time.Second * 60 * 60 * 4) //mysql 默认超时时间为 60*60*8=28800 SetConnMaxLifetime设置为小于数据库超时时间即可
	conn.Ping()

	session := conn.NewSession(nil)

	if migration {
		err = Migration(sqlDir, session)
		if err != nil {
			panic(err)
		}
	}

	return session
}

func Migration(sqlDir string, session *dbr.Session) error {
	migrations := &FileDirMigrationSource{
		Dir: sqlDir,
	}
	_, err := migrate.Exec(session.DB, "mysql", migrations, migrate.Up)
	if err != nil {
		return err
	}
	return nil
}

type byID []*migrate.Migration

func (b byID) Len() int           { return len(b) }
func (b byID) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byID) Less(i, j int) bool { return b[i].Less(b[j]) }

// FileDirMigrationSource 文件目录源 遇到目录进行递归获取
type FileDirMigrationSource struct {
	Dir string
}

// FindMigrations FindMigrations
func (f FileDirMigrationSource) FindMigrations() ([]*migrate.Migration, error) {
	filesystem := http.Dir(f.Dir)
	migrations := make([]*migrate.Migration, 0, 100)
	err := f.findMigrations(filesystem, &migrations)
	if err != nil {
		return nil, err
	}
	// Make sure migrations are sorted
	sort.Sort(byID(migrations))

	return migrations, nil
}

func (f FileDirMigrationSource) findMigrations(dir http.FileSystem, migrations *[]*migrate.Migration) error {

	file, err := dir.Open("/")
	if err != nil {
		return err
	}

	files, err := file.Readdir(0)
	if err != nil {
		return err
	}

	for _, info := range files {

		if strings.HasSuffix(info.Name(), ".sql") {
			file, err := dir.Open(info.Name())
			if err != nil {
				return fmt.Errorf("error while opening %s: %s", info.Name(), err)
			}

			migration, err := migrate.ParseMigration(info.Name(), file)
			if err != nil {
				return fmt.Errorf("error while parsing %s: %s", info.Name(), err)
			}
			*migrations = append(*migrations, migration)

		} else if info.IsDir() {
			err = f.findMigrations(http.Dir(fmt.Sprintf("%s/%s", f.Dir, info.Name())), migrations)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
