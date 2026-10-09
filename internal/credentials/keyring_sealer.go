package credentials

// Decrypter opens values sealed by the application keyring.
type Decrypter interface {
	Decrypt(ciphertext string) (string, error)
}

// SealKeyring is the application keyring surface a KeyringSealer needs.
type SealKeyring interface {
	Encrypter
	Decrypter
	PrimaryKeyID() string
}

// KeyringSealer adapts the application keyring to Sealer, reporting the
// primary key id used for each new ciphertext.
type KeyringSealer struct {
	keyring SealKeyring
}

// NewKeyringSealer wraps keyring as a Sealer.
func NewKeyringSealer(keyring SealKeyring) KeyringSealer {
	return KeyringSealer{keyring: keyring}
}

func (s KeyringSealer) Seal(plaintext string) (string, string, error) {
	ciphertext, err := s.keyring.Encrypt(plaintext)
	if err != nil {
		return "", "", err
	}
	return ciphertext, s.keyring.PrimaryKeyID(), nil
}

func (s KeyringSealer) Open(ciphertext string) (string, error) {
	return s.keyring.Decrypt(ciphertext)
}

var _ Sealer = KeyringSealer{}
