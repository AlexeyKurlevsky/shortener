package storage

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		err := s.Save("abc", "https://example.com")
		require.NoError(t, err)

		// проверяем, что FindIDByURL находит
		id, ok := s.FindIDByURL("https://example.com")
		assert.True(t, ok)
		assert.Equal(t, "abc", id)

		// перезаписываем abc -> new.com
		err = s.Save("abc", "https://new.com")
		require.NoError(t, err)

		// старый URL больше не должен находиться
		_, ok = s.FindIDByURL("https://example.com")
		assert.False(t, ok, "старый URL не должен быть найден после перезаписи")

		// новый URL должен находиться
		id, ok = s.FindIDByURL("https://new.com")
		assert.True(t, ok)
		assert.Equal(t, "abc", id)

		// проверяем, что Get возвращает новый URL
		val, err := s.Get("abc")
		assert.NoError(t, err)
		assert.Equal(t, "https://new.com", val)
	})
}

// Общий тест для всех хранилищ
func testStorage(t *testing.T, s Storage) {
	err := s.Save("abc", "https://example.com")
	assert.NoError(t, err)

	val, err := s.Get("abc")
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com", val)

	assert.True(t, s.Exists("abc"))
	assert.False(t, s.Exists("nonexistent"))

	_, err = s.Get("nonexistent")
	assert.Equal(t, ErrNotFound, err)

	err = s.Save("abc", "https://new.com")
	assert.NoError(t, err)
	val, err = s.Get("abc")
	assert.NoError(t, err)
	assert.Equal(t, "https://new.com", val)

	if js, ok := s.(*JSONStorage); ok {
		err = js.Save("def", "https://def.com")
		assert.NoError(t, err)

		s2, err := NewJSONStorage(js.filePath)
		assert.NoError(t, err)
		val2, err := s2.Get("def")
		assert.NoError(t, err)
		assert.Equal(t, "https://def.com", val2)
		val2, err = s2.Get("abc")
		assert.NoError(t, err)
		assert.Equal(t, "https://new.com", val2)
	}

	err = s.Save("abc", "https://example.com")
	assert.NoError(t, err)
	id, ok := s.FindIDByURL("https://example.com")
	assert.True(t, ok)
	assert.Equal(t, "abc", id)
	_, ok = s.FindIDByURL("https://nonexistent.com")
	assert.False(t, ok)
}
