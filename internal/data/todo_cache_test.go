package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"

	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// mockRedis is a simple mock implementing necessary redis.UniversalClient methods.
type mockRedis struct {
	redis.UniversalClient
	store map[string][]byte
}

func newMockRedis() *mockRedis {
	return &mockRedis{
		store: make(map[string][]byte),
	}
}

func (m *mockRedis) Get(ctx context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx)
	if val, ok := m.store[key]; ok {
		cmd.SetVal(string(val))
	} else {
		cmd.SetErr(redis.Nil)
	}
	return cmd
}

func (m *mockRedis) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(ctx)
	var byteVal []byte
	switch v := value.(type) {
	case []byte:
		byteVal = v
	case string:
		byteVal = []byte(v)
	default:
		var err error
		byteVal, err = json.Marshal(v)
		if err != nil {
			cmd.SetErr(err)
			return cmd
		}
	}
	m.store[key] = byteVal
	return cmd
}

func (m *mockRedis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	cmd := redis.NewIntCmd(ctx)
	var count int64
	for _, key := range keys {
		if _, ok := m.store[key]; ok {
			delete(m.store, key)
			count++
		}
	}
	cmd.SetVal(count)
	return cmd
}

func (m *mockRedis) Incr(ctx context.Context, key string) *redis.IntCmd {
	cmd := redis.NewIntCmd(ctx)
	valStr := string(m.store[key])
	var val int
	if valStr != "" {
		val, _ = strconv.Atoi(valStr)
	}
	val++
	m.store[key] = []byte(strconv.Itoa(val))
	cmd.SetVal(int64(val))
	return cmd
}

func TestCachedTodoRepo(t *testing.T) {
	// 1. Initialize SQLite in-memory DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	if err := db.AutoMigrate(&TodoModel{}); err != nil {
		t.Fatalf("failed to migrate schema: %v", err)
	}

	// 2. Setup repo and mock redis
	rawRepo := &todoRepo{db: db}
	rdb := newMockRedis()
	repo := &cachedTodoRepo{
		TodoRepo: rawRepo,
		redis:    rdb,
	}

	ctx := context.Background()

	// 3. Test Create & List Cache Invalidation
	todo := &biz.Todo{
		Title:   "test cache",
		Content: "learn decorators",
	}
	created, err := repo.CreateTodo(ctx, todo)
	if err != nil {
		t.Fatalf("CreateTodo failed: %v", err)
	}

	// Verify list cache is invalid/empty first
	listKey := repo.listCacheKey(ctx, biz.ListOptions{Limit: 20})
	if _, ok := rdb.store[listKey]; ok {
		t.Fatal("expected list cache to be empty/invalidated on creation")
	}

	// 4. Test List Caching
	todos, err := repo.ListTodos(ctx)
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) != 1 || todos[0].ID != created.ID {
		t.Fatalf("unexpected list output: %v", todos)
	}

	// Verify list cache was populated
	if _, ok := rdb.store[listKey]; !ok {
		t.Fatal("expected list cache to be populated after query")
	}

	// 5. Test FindByID Cache Hit & Miss
	singleKey := todoKey(created.ID)
	// Cache should be empty initially
	if _, ok := rdb.store[singleKey]; ok {
		t.Fatal("expected single cache to be empty initially")
	}

	// First load -> Cache Miss -> Populate
	got1, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if got1.Title != "test cache" {
		t.Fatalf("unexpected title: %q", got1.Title)
	}

	// Verify single cache populated
	if _, ok := rdb.store[singleKey]; !ok {
		t.Fatal("expected single cache to be populated after FindByID")
	}

	// Second load -> Cache Hit
	// Manually corrupt cache values to verify it reads from cache
	rdb.store[singleKey] = []byte(`{"ID":1,"Title":"cached title","Content":"cached content"}`)
	got2, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID on cache hit failed: %v", err)
	}
	if got2.Title != "cached title" {
		t.Fatalf("expected to read from cache, got: %q", got2.Title)
	}

	// 6. Test Cache Penetration Protection (Null Cache)
	nonExistentKey := todoKey(9999)
	if _, ok := rdb.store[nonExistentKey]; ok {
		t.Fatal("expected non-existent cache key to be empty")
	}

	// Load non-existent ID -> Cache Miss -> Cache Null
	_, err = repo.FindByID(ctx, 9999)
	if err != biz.ErrTodoNotFound {
		t.Fatalf("expected ErrTodoNotFound, got: %v", err)
	}

	// Verify "null" cache value populated
	if val := string(rdb.store[nonExistentKey]); val != "null" {
		t.Fatalf("expected null cache value, got: %q", val)
	}

	// Subsequent queries should hit the null cache immediately
	_, err = repo.FindByID(ctx, 9999)
	if err != biz.ErrTodoNotFound {
		t.Fatalf("expected ErrTodoNotFound from null cache, got: %v", err)
	}

	// 7. Test Update Cache Invalidation
	created.Title = "updated title"
	_, err = repo.UpdateTodo(ctx, created)
	if err != nil {
		t.Fatalf("UpdateTodo failed: %v", err)
	}

	// Single cache and list cache should be cleared/invalidated
	if _, ok := rdb.store[singleKey]; ok {
		t.Fatal("expected single cache key to be deleted on update")
	}
	// The new list version should have changed, meaning listKey will be different
	newListKey := repo.listCacheKey(ctx, biz.ListOptions{Limit: 20})
	if newListKey == listKey {
		t.Fatal("expected list cache version key to increment on update")
	}
}

func TestTodoRepoMapsStoreErrors(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	if err := db.AutoMigrate(&TodoModel{}); err != nil {
		t.Fatalf("failed to migrate schema: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("failed to close sql db: %v", err)
	}

	repo := &todoRepo{db: db}
	ctx := context.Background()

	todo := &biz.Todo{ID: 1, Title: "store error"}
	cases := []struct {
		name string
		run  func() error
	}{
		{
			name: "find",
			run: func() error {
				_, err := repo.FindByID(ctx, 1)
				return err
			},
		},
		{
			name: "list",
			run: func() error {
				_, err := repo.ListTodos(ctx)
				return err
			},
		},
		{
			name: "create",
			run: func() error {
				_, err := repo.CreateTodo(ctx, todo)
				return err
			},
		},
		{
			name: "update",
			run: func() error {
				_, err := repo.UpdateTodo(ctx, todo)
				return err
			},
		},
		{
			name: "delete",
			run: func() error {
				return repo.DeleteTodo(ctx, 1)
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err != biz.ErrTodoInternal {
				t.Fatalf("expected ErrTodoInternal, got %v", err)
			}
		})
	}

	if err := mapTodoStoreError(sql.ErrNoRows); err != biz.ErrTodoInternal {
		t.Fatalf("expected unknown store errors to map to ErrTodoInternal, got %v", err)
	}
}
