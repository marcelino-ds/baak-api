package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/yafyx/baak-api/utils"
)

func cacheKey(namespace string, parts ...string) string {
	value := namespace + "\x00" + strings.Join(parts, "\x00")
	hash := sha256.Sum256([]byte(value))
	return namespace + ":" + hex.EncodeToString(hash[:])
}

func cachedValue[T any](
	ctx context.Context,
	enabled bool,
	ttl time.Duration,
	key string,
	load func(context.Context) (T, error),
) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if !enabled || ttl <= 0 {
		return load(ctx)
	}

	cache := utils.GetCache()
	value, err := cache.GetOrLoad(
		ctx,
		key,
		ttl,
		func(ctx context.Context) (interface{}, error) { return load(ctx) },
	)
	if err != nil {
		return zero, err
	}
	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("unexpected cached data type for %s", key)
	}
	return typed, nil
}
