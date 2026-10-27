package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

func randomHex(n int) string {
	b := make([]byte, n)
	_, err := rand.Read(b)
	check(err)
	return hex.EncodeToString(b)
}
func newID() string { return randomHex(16) }
func hashPassword(password string) string {
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	check(err)
	key := pbkdf2.Key([]byte(password), salt, 600000, 32, sha256.New)
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(key)
}
func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, ":")
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[1])
	if err != nil || len(want) != 32 {
		return false
	}
	got := pbkdf2.Key([]byte(password), salt, 600000, 32, sha256.New)
	return subtle.ConstantTimeCompare(want, got) == 1
}
func digest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func requestToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}
	if c, err := r.Cookie("linji_session"); err == nil {
		return c.Value
	}
	return ""
}
func (a *App) currentUser(r *http.Request) User {
	token := requestToken(r)
	if token == "" {
		fail(401, "请先登录")
	}
	u := User{}
	err := a.DB.QueryRow("SELECT u.id,u.username,u.display_name FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>?", digest(token), time.Now().Unix()).Scan(&u.ID, &u.Username, &u.DisplayName)
	if err == sql.ErrNoRows {
		fail(401, "请先登录，或重新登录已过期的会话")
	}
	check(err)
	return u
}
func (a *App) createSession(w http.ResponseWriter, userID int64) string {
	token := randomHex(48)
	_, err := a.DB.Exec("DELETE FROM sessions WHERE expires_at<=?", time.Now().Unix())
	check(err)
	_, err = a.DB.Exec("INSERT INTO sessions(token_hash,user_id,expires_at) VALUES (?,?,?)", digest(token), userID, time.Now().Add(7*24*time.Hour).Unix())
	check(err)
	http.SetCookie(w, &http.Cookie{Name: "linji_session", Value: token, Path: "/", MaxAge: 7 * 86400, HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteStrictMode})
	return token
}
func withTx[T any](a *App, fn func(*sql.Tx) T) T {
	tx, err := a.DB.Begin()
	check(err)
	defer tx.Rollback()
	out := fn(tx)
	check(tx.Commit())
	return out
}
func (a *App) rateLimit(r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	ip = cut(ip, 64)
	timestamp := time.Now().Unix()
	allowed := withTx(a, func(tx *sql.Tx) bool {
		_, err := tx.Exec("INSERT IGNORE INTO login_attempts(ip,attempts,started) VALUES (?,0,?)", ip, timestamp)
		check(err)
		var count int
		var started int64
		check(tx.QueryRow("SELECT attempts,started FROM login_attempts WHERE ip=? FOR UPDATE", ip).Scan(&count, &started))
		if timestamp-started >= 300 {
			count = 0
			started = timestamp
		}
		if count >= 15 {
			return false
		}
		_, err = tx.Exec("UPDATE login_attempts SET attempts=?,started=? WHERE ip=?", count+1, started, ip)
		check(err)
		return true
	})
	if !allowed {
		fail(429, "尝试次数过多，请 5 分钟后重试")
	}
}
func (a *App) setup(w http.ResponseWriter, r *http.Request) {
	a.rateLimit(r)
	var in Credentials
	decode(r, &in)
	in.validate()
	hashed := hashPassword(in.Password)
	user := withTx(a, func(tx *sql.Tx) User {
		var lock int
		check(tx.QueryRow("SELECT id FROM app_state WHERE id=1 FOR UPDATE").Scan(&lock))
		var count int
		check(tx.QueryRow("SELECT COUNT(*) FROM users").Scan(&count))
		if count > 0 {
			fail(409, "已完成初始化，请登录")
		}
		result, err := tx.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,?,?,?)", in.Username, hashed, in.DisplayName, now())
		check(err)
		id, err := result.LastInsertId()
		check(err)
		return User{id, in.Username, in.DisplayName}
	})
	token := a.createSession(w, user.ID)
	respond(w, 201, map[string]any{"token": token, "user": user})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	a.rateLimit(r)
	var in Credentials
	decode(r, &in)
	in.validate()
	var u User
	var stored string
	err := a.DB.QueryRow("SELECT id,username,display_name,password_hash FROM users WHERE username=?", in.Username).Scan(&u.ID, &u.Username, &u.DisplayName, &stored)
	if err != nil && err != sql.ErrNoRows {
		check(err)
	}
	if err == sql.ErrNoRows || !verifyPassword(in.Password, stored) {
		fail(401, "用户名或密码错误")
	}
	respond(w, 200, map[string]any{"token": a.createSession(w, u.ID), "user": u})
}
