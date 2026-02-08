package api

import (
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/cryguy/hostedat/internal/auth"
	"github.com/cryguy/hostedat/internal/models"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB        *gorm.DB
	JWTSecret string
}

type registerRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code,omitempty"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string      `json:"token"`
	User  models.User `json:"user"`
}

func (h *AuthHandler) Register(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		return errorJSON(c, http.StatusBadRequest, "email and password are required")
	}
	if len(req.Password) < 8 {
		return errorJSON(c, http.StatusBadRequest, "password must be at least 8 characters")
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid email format")
	}

	// Check registration settings
	regEnabled, err := models.GetSetting(h.DB, "registration_enabled")
	if err != nil || regEnabled != "true" {
		return errorJSON(c, http.StatusForbidden, "registration is disabled")
	}

	// Check invite requirement
	var inviteID *string
	inviteRequired, _ := models.GetSetting(h.DB, "invite_required")
	if inviteRequired == "true" {
		if req.InviteCode == "" {
			return errorJSON(c, http.StatusBadRequest, "invite code is required")
		}

		var invite models.Invite
		if err := h.DB.Where("code = ? AND active = ?", req.InviteCode, true).First(&invite).Error; err != nil {
			return errorJSON(c, http.StatusBadRequest, "invalid invite code")
		}

		if invite.ExpiresAt != nil && invite.ExpiresAt.Before(time.Now()) {
			return errorJSON(c, http.StatusBadRequest, "invite code has expired")
		}

		if invite.MaxUses != nil && invite.UseCount >= *invite.MaxUses {
			return errorJSON(c, http.StatusBadRequest, "invite code has reached max uses")
		}

		inviteID = &invite.ID
	}

	// Check for duplicate email
	var existing models.User
	if err := h.DB.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		return errorJSON(c, http.StatusConflict, "email already registered")
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to hash password")
	}

	// First user becomes superadmin
	var userCount int64
	h.DB.Model(&models.User{}).Count(&userCount)
	role := "user"
	if userCount == 0 {
		role = "superadmin"
	}

	user := models.User{
		Email:        req.Email,
		PasswordHash: hash,
		Role:         role,
		InvitedBy:    inviteID,
	}

	if err := h.DB.Create(&user).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to create user")
	}

	// Increment invite use count
	if inviteID != nil {
		h.DB.Model(&models.Invite{}).Where("id = ?", *inviteID).UpdateColumn("use_count", gorm.Expr("use_count + 1"))
	}

	token, err := auth.GenerateToken(user.ID, user.Email, user.Role, h.JWTSecret)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusCreated, authResponse{Token: token, User: user})
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		return errorJSON(c, http.StatusBadRequest, "email and password are required")
	}

	var user models.User
	if err := h.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return errorJSON(c, http.StatusUnauthorized, "invalid credentials")
	}

	if err := auth.CheckPassword(req.Password, user.PasswordHash); err != nil {
		return errorJSON(c, http.StatusUnauthorized, "invalid credentials")
	}

	token, err := auth.GenerateToken(user.ID, user.Email, user.Role, h.JWTSecret)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusOK, authResponse{Token: token, User: user})
}

func (h *AuthHandler) Logout(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"message": "logged out"})
}

func validatePort(port string) (int, error) {
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("invalid port")
	}
	return p, nil
}

// CLILogin serves a self-contained HTML login form for CLI authentication.
// GET /api/v1/auth/cli?port=PORT&state=STATE&code_challenge=CHALLENGE&code_challenge_method=S256
func (h *AuthHandler) CLILogin(c echo.Context) error {
	port := c.QueryParam("port")
	state := c.QueryParam("state")
	if port == "" || state == "" {
		return errorJSON(c, http.StatusBadRequest, "port and state are required")
	}
	portNum, err := validatePort(port)
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "port must be a number between 1 and 65535")
	}

	codeChallenge := c.QueryParam("code_challenge")
	codeChallengeMethod := c.QueryParam("code_challenge_method")
	if codeChallenge == "" {
		return errorJSON(c, http.StatusBadRequest, "code_challenge is required")
	}
	if codeChallengeMethod != "S256" {
		return errorJSON(c, http.StatusBadRequest, "code_challenge_method must be S256")
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>hostedat — CLI Login</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,-apple-system,sans-serif;background:#0a0a0a;color:#e5e5e5;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#171717;border:1px solid #262626;border-radius:12px;padding:2rem;width:100%%;max-width:380px}
h1{font-size:1.25rem;margin-bottom:1.5rem;text-align:center}
label{display:block;font-size:.875rem;margin-bottom:.25rem;color:#a3a3a3}
input{width:100%%;padding:.625rem .75rem;background:#0a0a0a;border:1px solid #404040;border-radius:6px;color:#e5e5e5;font-size:.875rem;margin-bottom:1rem}
input:focus{outline:none;border-color:#3b82f6}
button{width:100%%;padding:.625rem;background:#3b82f6;color:#fff;border:none;border-radius:6px;font-size:.875rem;cursor:pointer;font-weight:500}
button:hover{background:#2563eb}
.error{color:#ef4444;font-size:.8rem;margin-bottom:.75rem;display:none}
</style>
</head>
<body>
<div class="card">
<h1>Sign in to hostedat</h1>
<div class="error" id="err"></div>
<form id="f">
<label for="email">Email</label>
<input type="email" id="email" name="email" required autofocus>
<label for="password">Password</label>
<input type="password" id="password" name="password" required>
<button type="submit">Sign in</button>
</form>
</div>
<script>
document.getElementById('f').addEventListener('submit',async function(e){
e.preventDefault();
const errEl=document.getElementById('err');
errEl.style.display='none';
try{
const r=await fetch('/api/v1/auth/cli',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({email:document.getElementById('email').value,password:document.getElementById('password').value,port:"%d",state:%q,code_challenge:%q})});
const d=await r.json();
if(!r.ok){errEl.textContent=d.error||'Login failed';errEl.style.display='block';return}
window.location.href=d.redirect;
}catch(ex){errEl.textContent='Network error';errEl.style.display='block'}
});
</script>
</body>
</html>`, portNum, state, codeChallenge)

	return c.HTML(http.StatusOK, html)
}

type cliLoginSubmitRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	Port          string `json:"port"`
	State         string `json:"state"`
	CodeChallenge string `json:"code_challenge"`
}

// CLILoginSubmit validates credentials and returns a redirect URL with an auth code.
// POST /api/v1/auth/cli
func (h *AuthHandler) CLILoginSubmit(c echo.Context) error {
	var req cliLoginSubmitRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		return errorJSON(c, http.StatusBadRequest, "email and password are required")
	}
	if req.Port == "" || req.State == "" {
		return errorJSON(c, http.StatusBadRequest, "port and state are required")
	}
	if req.CodeChallenge == "" {
		return errorJSON(c, http.StatusBadRequest, "code_challenge is required")
	}
	portNum, err := validatePort(req.Port)
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "port must be a number between 1 and 65535")
	}

	var user models.User
	if err := h.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return errorJSON(c, http.StatusUnauthorized, "invalid credentials")
	}

	if err := auth.CheckPassword(req.Password, user.PasswordHash); err != nil {
		return errorJSON(c, http.StatusUnauthorized, "invalid credentials")
	}

	code, err := auth.GenerateAuthCode()
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to generate auth code")
	}

	authCode := models.AuthCode{
		Code:          code,
		UserID:        user.ID,
		CodeChallenge: req.CodeChallenge,
		ExpiresAt:     time.Now().Add(60 * time.Second),
	}
	if err := h.DB.Create(&authCode).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to store auth code")
	}

	redirectURL := fmt.Sprintf("http://localhost:%d/callback?code=%s&state=%s", portNum, code, req.State)

	return c.JSON(http.StatusOK, map[string]string{"redirect": redirectURL})
}

type tokenExchangeRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
}

// TokenExchange exchanges an authorization code + PKCE code verifier for a JWT.
// POST /api/v1/auth/token
func (h *AuthHandler) TokenExchange(c echo.Context) error {
	var req tokenExchangeRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	if req.Code == "" || req.CodeVerifier == "" {
		return errorJSON(c, http.StatusBadRequest, "code and code_verifier are required")
	}

	var authCode models.AuthCode
	if err := h.DB.Where("code = ?", req.Code).First(&authCode).Error; err != nil {
		return errorJSON(c, http.StatusUnauthorized, "invalid authorization code")
	}

	if authCode.Used {
		return errorJSON(c, http.StatusUnauthorized, "authorization code already used")
	}

	if time.Now().After(authCode.ExpiresAt) {
		return errorJSON(c, http.StatusUnauthorized, "authorization code expired")
	}

	if !auth.VerifyCodeChallenge(req.CodeVerifier, authCode.CodeChallenge) {
		return errorJSON(c, http.StatusUnauthorized, "code verifier mismatch")
	}

	// Atomically mark as used — check RowsAffected for race safety
	result := h.DB.Model(&models.AuthCode{}).
		Where("id = ? AND used = ?", authCode.ID, false).
		Update("used", true)
	if result.RowsAffected == 0 {
		return errorJSON(c, http.StatusUnauthorized, "authorization code already used")
	}

	var user models.User
	if err := h.DB.First(&user, "id = ?", authCode.UserID).Error; err != nil {
		return errorJSON(c, http.StatusInternalServerError, "user not found")
	}

	token, err := auth.GenerateToken(user.ID, user.Email, user.Role, h.JWTSecret)
	if err != nil {
		return errorJSON(c, http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusOK, map[string]string{"token": token})
}
