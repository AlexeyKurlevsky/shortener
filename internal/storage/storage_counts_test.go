package storage

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCounts_Memory(t *testing.T) {
	testCounts(t, NewMemoryStorage())
}

func TestCounts_JSON(t *testing.T) {
	tmp, err := os.CreateTemp("", "storage-counts*.json")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())
	tmp.Close()

	s, err := NewJSONStorage(tmp.Name())
	require.NoError(t, err)
	testCounts(t, s)
}

// testCounts проверяет семантику CountURLs/CountUsers, общую для всех
// бэкендов: urls считает только активные записи, users — DISTINCT user_id
// по всем записям (включая soft-deleted).
func testCounts(t *testing.T, s Storage) {
	ctx := context.Background()

	t.Run("empty storage", func(t *testing.T) {
		urls, err := s.CountURLs(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, urls)

		users, err := s.CountUsers(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, users)
	})

	t.Run("single user, two urls", func(t *testing.T) {
		require.NoError(t, s.Save(ctx, "a1", "https://a1.com", "user1"))
		require.NoError(t, s.Save(ctx, "a2", "https://a2.com", "user1"))

		urls, err := s.CountURLs(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, urls)

		users, err := s.CountUsers(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, users, "два url одного юзера — это один уникальный user")
	})

	t.Run("second user", func(t *testing.T) {
		require.NoError(t, s.Save(ctx, "b1", "https://b1.com", "user2"))

		urls, err := s.CountURLs(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, urls)

		users, err := s.CountUsers(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, users)
	})

	t.Run("soft delete reduces urls but keeps user", func(t *testing.T) {
		require.NoError(t, s.DeleteURLs(ctx, []string{"a1"}, "user1"))

		urls, err := s.CountURLs(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, urls, "удалённая ссылка не должна считаться в urls")

		users, err := s.CountUsers(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, users, "пользователь не исчезает вместе со ссылкой")
	})

	t.Run("batch save adds urls and a new user", func(t *testing.T) {
		require.NoError(t, s.BatchSave(ctx, []BatchItem{
			{ID: "c1", URL: "https://c1.com"},
			{ID: "c2", URL: "https://c2.com"},
		}, "user3"))

		urls, err := s.CountURLs(ctx)
		require.NoError(t, err)
		assert.Equal(t, 4, urls)

		users, err := s.CountUsers(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, users)
	})

	t.Run("user stays after all their urls deleted", func(t *testing.T) {
		require.NoError(t, s.DeleteURLs(ctx, []string{"b1"}, "user2"))

		urls, err := s.CountURLs(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, urls)

		users, err := s.CountUsers(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, users, "user2 всё ещё учитывается, хоть у него и нет активных ссылок")
	})
}
