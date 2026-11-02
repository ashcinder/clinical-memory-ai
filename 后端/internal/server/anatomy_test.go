package server

import "testing"

func TestAnatomySharedAndConflict(t *testing.T) {
	a := integrationApp(t)
	c := &testAPI{t: t, handler: a.Handler()}
	c.request("GET", "/api/anatomy", nil, 401)
	setup := c.request("POST", "/api/auth/setup", Credentials{Username: "atlas_qa", Password: "atlas-test-pass", DisplayName: "图谱测试"}, 201)
	c.token = getString(setup, "token")
	first := c.request("GET", "/api/anatomy", nil, 200)
	p := AnatomyPoint{ID: "point-test", Name: "测试标注", Position: [3]float64{0, 1, .2}}
	saved := c.request("PUT", "/api/anatomy", AnatomyAtlas{getString(first, "revision"), []AnatomyPoint{p}}, 200)
	c.request("PUT", "/api/anatomy", AnatomyAtlas{getString(first, "revision"), []AnatomyPoint{p}}, 409)
	p.Position[0] = 4
	c.request("PUT", "/api/anatomy", AnatomyAtlas{getString(saved, "revision"), []AnatomyPoint{p}}, 422)
	p.Position[0] = 0
	c.request("PUT", "/api/anatomy", AnatomyAtlas{getString(saved, "revision"), []AnatomyPoint{p, p}}, 422)
	otherID, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES ('atlas_other','unused','Other',?)", now())
	check(err)
	uid, _ := otherID.LastInsertId()
	token := randomHex(48)
	_, err = a.DB.Exec("INSERT INTO sessions(token_hash,user_id,expires_at) VALUES (?,?,4102444800)", digest(token), uid)
	check(err)
	other := &testAPI{t: t, handler: a.Handler(), token: token}
	got := other.request("GET", "/api/anatomy", nil, 200)
	if getString(got, "revision") != getString(saved, "revision") {
		t.Fatal("shared atlas mismatch")
	}
	other.request("PUT", "/api/anatomy", AnatomyAtlas{getString(saved, "revision"), []AnatomyPoint{}}, 200)
}
