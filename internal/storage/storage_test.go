package storage

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testUserID = "test-user"

func TestMemoryStorage(t *testing.T) {
	s := NewMemoryStorage()
	testStorage(t, s)
}

func TestJSONStorage(t *testing.T) {
	tmp, err := os.CreateTemp("", "storage*.json")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())
	tmp.Close()

	s, err := NewJSONStorage(tmp.Name())
	require.NoError(t, err)
	testStorage(t, s)

	// Дополнительно проверяем корректность urlMap при перезаписи
	t.Run("overwrite and urlMap", func(t *testing.T) {
		// сохраняем abc -> example.com
		err := s.Save(context.Background(), "abc", "https://example.com", testUserID)
		require.NoError(t, err)

		// проверяем, что FindIDByURL находит
		id, ok := s.FindIDByURL(context.Background(), "https://example.com")
		assert.True(t, ok)
		assert.Equal(t, "abc", id)

		// перезаписываем abc -> new.com
		err = s.Save(context.Background(), "abc", "https://new.com", testUserID)
		require.NoError(t, err)

		// старый URL больше не должен находиться
		_, ok = s.FindIDByURL(context.Background(), "https://example.com")
		assert.False(t, ok, "старый URL не должен быть найден после перезаписи")

		// новый URL должен находиться
		id, ok = s.FindIDByURL(context.Background(), "https://new.com")
		assert.True(t, ok)
		assert.Equal(t, "abc", id)

		// проверяем, что Get возвращает новый URL
		val, err := s.Get(context.Background(), "abc")
		assert.NoError(t, err)
		assert.Equal(t, "https://new.com", val)
	})
}

// Общий тест для всех хранилищ
func testStorage(t *testing.T, s Storage) {
	err := s.Save(context.Background(), "abc", "https://example.com", testUserID)
	assert.NoError(t, err)

	val, err := s.Get(context.Background(), "abc")
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com", val)

	assert.True(t, s.Exists(context.Background(), "abc"))
	assert.False(t, s.Exists(context.Background(), "nonexistent"))

	_, err = s.Get(context.Background(), "nonexistent")
	assert.Equal(t, ErrNotFound, err)

	err = s.Save(context.Background(), "abc", "https://new.com", testUserID)
	assert.NoError(t, err)
	val, err = s.Get(context.Background(), "abc")
	assert.NoError(t, err)
	assert.Equal(t, "https://new.com", val)

	if js, ok := s.(*JSONStorage); ok {
		err = js.Save(context.Background(), "def", "https://def.com", testUserID)
		assert.NoError(t, err)

		s2, err := NewJSONStorage(js.filePath)
		assert.NoError(t, err)
		val2, err := s2.Get(context.Background(), "def")
		assert.NoError(t, err)
		assert.Equal(t, "https://def.com", val2)
		val2, err = s2.Get(context.Background(), "abc")
		assert.NoError(t, err)
		assert.Equal(t, "https://new.com", val2)
	}

	t.Run("BatchSave", func(t *testing.T) {
		items := []BatchItem{
			{ID: "batch1", URL: "https://batch1.com"},
			{ID: "batch2", URL: "https://batch2.com"},
		}
		err := s.BatchSave(context.Background(), items, testUserID)
		assert.NoError(t, err)

		val, err := s.Get(context.Background(), "batch1")
		assert.NoError(t, err)
		assert.Equal(t, "https://batch1.com", val)

		val, err = s.Get(context.Background(), "batch2")
		assert.NoError(t, err)
		assert.Equal(t, "https://batch2.com", val)

		id, ok := s.FindIDByURL(context.Background(), "https://batch1.com")
		assert.True(t, ok)
		assert.Equal(t, "batch1", id)
	})

	err = s.Save(context.Background(), "abc", "https://example.com", testUserID)
	assert.NoError(t, err)
	id, ok := s.FindIDByURL(context.Background(), "https://example.com")
	assert.True(t, ok)
	assert.Equal(t, "abc", id)
	_, ok = s.FindIDByURL(context.Background(), "https://nonexistent.com")
	assert.False(t, ok)
}
