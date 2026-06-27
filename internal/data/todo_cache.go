package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kratos/kratos-layout/internal/biz"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	todoCacheTTL       = 5 * time.Minute
	todoListCacheTTL   = 30 * time.Second
	todoNullCacheTTL   = 1 * time.Minute // TTL for non-existent records to prevent cache penetration
	todoListVersionKey = "todo:list:version"
)

// cachedTodoRepo wraps a TodoRepo with Redis cache-aside.
// By embedding biz.TodoRepo anonymous interface, Go automatically delegates
// any un-implemented methods (like CreateTodo, UpdateTodo, DeleteTodo) to the underlying repo.
type cachedTodoRepo struct {
	biz.TodoRepo
	redis redis.UniversalClient
	sg    singleflight.Group
}

// todoKey builds a cache key for a single todo.
func todoKey(id int64) string {
	return fmt.Sprintf("todo:%d", id)
}

// getListVersion returns the current list version, default to "1" on error/not found.
func (r *cachedTodoRepo) getListVersion(ctx context.Context) string {
	version, err := r.redis.Get(ctx, todoListVersionKey).Result()
	if err != nil || version == "" {
		return "1"
	}
	return version
}

// incrListVersion increments the list version to invalidate all list query caches.
func (r *cachedTodoRepo) incrListVersion(ctx context.Context) {
	_ = r.redis.Incr(ctx, todoListVersionKey).Err()
}

// listCacheKey builds a cache key for a list query incorporating the current version.
func (r *cachedTodoRepo) listCacheKey(ctx context.Context, opts biz.ListOptions) string {
	var comp string
	if opts.Completed != nil {
		comp = fmt.Sprintf("%v", *opts.Completed)
	} else {
		comp = "nil"
	}
	version := r.getListVersion(ctx)
	return fmt.Sprintf("todo:list:%s:%s:%s:%s:%d:%d", version, comp, opts.Search, opts.OrderBy, opts.Offset, opts.Limit)
}

// FindByID returns a todo by ID, checking cache first.
func (r *cachedTodoRepo) FindByID(ctx context.Context, id int64) (*biz.Todo, error) {
	key := todoKey(id)

	// Try cache
	data, err := r.redis.Get(ctx, key).Bytes()
	if err == nil {
		if string(data) == "null" {
			return nil, biz.ErrTodoNotFound
		}
		var todo biz.Todo
		if err := json.Unmarshal(data, &todo); err == nil {
			return &todo, nil
		}
		// Corrupt cache entry — delete and fall through
		_ = r.redis.Del(ctx, key).Err()
	}

	// Cache miss — use singleflight to merge concurrent DB fetches
	v, err, _ := r.sg.Do(key, func() (any, error) {
		todo, err := r.TodoRepo.FindByID(ctx, id)
		if err != nil {
			if err == biz.ErrTodoNotFound {
				// Cache empty/null value to prevent cache penetration
				_ = r.redis.Set(ctx, key, "null", todoNullCacheTTL).Err()
			}
			return nil, err
		}

		// Populate cache
		if data, err := json.Marshal(todo); err == nil {
			_ = r.redis.Set(ctx, key, data, todoCacheTTL).Err()
		}

		return todo, nil
	})

	if err != nil {
		return nil, err
	}
	return v.(*biz.Todo), nil
}

// ListTodos lists todos, checking cache first.
func (r *cachedTodoRepo) ListTodos(ctx context.Context, opts ...biz.ListOption) ([]*biz.Todo, error) {
	options := biz.ListOptions{Limit: 20}
	for _, opt := range opts {
		opt(&options)
	}

	key := r.listCacheKey(ctx, options)

	// Try cache
	data, err := r.redis.Get(ctx, key).Bytes()
	if err == nil {
		var todos []*biz.Todo
		if err := json.Unmarshal(data, &todos); err == nil {
			return todos, nil
		}
		_ = r.redis.Del(ctx, key).Err()
	}

	// Cache miss — use singleflight to merge concurrent DB fetches
	v, err, _ := r.sg.Do(key, func() (any, error) {
		todos, err := r.TodoRepo.ListTodos(ctx, opts...)
		if err != nil {
			return nil, err
		}

		// Populate cache
		if data, err := json.Marshal(todos); err == nil {
			_ = r.redis.Set(ctx, key, data, todoListCacheTTL).Err()
		}

		return todos, nil
	})

	if err != nil {
		return nil, err
	}
	return v.([]*biz.Todo), nil
}

// UpdateTodo updates a todo, invalidating single cache and list version cache.
func (r *cachedTodoRepo) UpdateTodo(ctx context.Context, todo *biz.Todo) (*biz.Todo, error) {
	result, err := r.TodoRepo.UpdateTodo(ctx, todo)
	if err != nil {
		return nil, err
	}
	// Invalidate cached single item
	_ = r.redis.Del(ctx, todoKey(todo.ID)).Err()
	// Invalidate all list query caches
	r.incrListVersion(ctx)
	return result, nil
}

// DeleteTodo deletes a todo, invalidating single cache and list version cache.
func (r *cachedTodoRepo) DeleteTodo(ctx context.Context, id int64) error {
	if err := r.TodoRepo.DeleteTodo(ctx, id); err != nil {
		return err
	}
	// Invalidate cached single item
	_ = r.redis.Del(ctx, todoKey(id)).Err()
	// Invalidate all list query caches
	r.incrListVersion(ctx)
	return nil
}

// CreateTodo creates a todo and invalidates the list version cache.
func (r *cachedTodoRepo) CreateTodo(ctx context.Context, todo *biz.Todo) (*biz.Todo, error) {
	result, err := r.TodoRepo.CreateTodo(ctx, todo)
	if err != nil {
		return nil, err
	}
	// Invalidate all list query caches (since list has changed)
	r.incrListVersion(ctx)
	return result, nil
}
