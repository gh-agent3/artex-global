package db

import "database/sql"

// Settings is a tiny key-value store for global app config the UI toggles at
// runtime (e.g. traffic_capture). Missing keys fall back to caller defaults.

// settingsSecretKeys lists the setting keys whose values are credentials. Their
// values ride the same at-rest encryption as llm_profiles.api_key
// (credential_crypto.go): ciphertext carries the enc:v1: prefix once a master
// key is installed, plaintext until then (lazy upgrade on next save). Keep in
// sync with the key constants in server/manager.go (settingBraveKey/settingTavilyKey).
var settingsSecretKeys = map[string]bool{
	"brave_search_api_key":  true,
	"tavily_search_api_key": true,
}

// GetSetting returns the stored value and ok=false when the key is unset.
// Credential values are transparently decrypted here; values without the
// enc:v1: prefix (legacy plaintext, or written while no master key was
// installed) come back untouched. A ciphertext row read without its master key
// is an error, not an empty string — an empty key would surface later as an
// unexplainable 401 from the search provider.
func (d *DB) GetSetting(key string) (value string, ok bool, err error) {
	err = d.QueryRow(`SELECT value FROM settings WHERE key=$1`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if value, err = decryptCredential(value); err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetSetting upserts a setting value. Values under settingsSecretKeys are
// encrypted at rest when a master key is installed (no-op passthrough when it
// is not — see encryptCredential).
func (d *DB) SetSetting(key, value string) error {
	if settingsSecretKeys[key] {
		var err error
		if value, err = encryptCredential(value); err != nil {
			return err
		}
	}
	_, err := d.Exec(`
INSERT INTO settings(key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, value)
	return err
}

// InsertSettingIfAbsent 는 key 가 아직 없을 때만 써넣고, 이미 있으면 값을 그대로 두고
// inserted=false 를 돌려준다. auth.password_hash 처럼 "최초 1회만 설정" 이어야 하는 키에
// 쓴다: 판정을 호출자의 GetSetting 검사에 맡기면 읽기 오류나 동시 요청(bcrypt 가 수십
// 밀리초를 쓴다)에 그대로 뚫리므로, 보장을 데이터베이스 기본키 제약에 내린다.
func (d *DB) InsertSettingIfAbsent(key, value string) (inserted bool, err error) {
	res, err := d.Exec(`
INSERT INTO settings(key, value) VALUES ($1, $2)
ON CONFLICT (key) DO NOTHING`, key, value)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// GetBool returns the boolean setting, or def when unset/unparseable.
func (d *DB) GetBool(key string, def bool) bool {
	v, ok, err := d.GetSetting(key)
	if err != nil || !ok {
		return def
	}
	return v == "true" || v == "1"
}

// SetBool stores a boolean setting as "true"/"false".
func (d *DB) SetBool(key string, val bool) error {
	if val {
		return d.SetSetting(key, "true")
	}
	return d.SetSetting(key, "false")
}
