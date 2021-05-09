// +build !nacl,!arm

package stingle

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesruan/sodium"
)

func TestFileEncryption(t *testing.T) {
	sk := MakeSecretKey()
	mk := sodium.MakeMasterKey()

	header := Header{
		FileID:       []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"),
		Version:      1,
		ChunkSize:    128,
		SymmetricKey: []byte(mk.Bytes),
	}

	orig := []byte(`Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Lorem ipsum dolor sit amet consectetur adipiscing. Porttitor lacus luctus accumsan tortor posuere ac ut consequat semper. Pulvinar etiam non quam lacus suspendisse faucibus interdum. Elementum facilisis leo vel fringilla est ullamcorper eget nulla. Cursus turpis massa tincidunt dui ut ornare lectus sit. Amet consectetur adipiscing elit duis tristique. Sed tempus urna et pharetra. Cursus metus aliquam eleifend mi in. Vulputate dignissim suspendisse in est ante in. Ultricies lacus sed turpis tincidunt id aliquet. Faucibus nisl tincidunt eget nullam. Sit amet commodo nulla facilisi nullam vehicula ipsum. Volutpat blandit aliquam etiam erat velit scelerisque in dictum non. Condimentum lacinia quis vel eros donec ac odio. Mattis nunc sed blandit libero volutpat. Lectus sit amet est placerat in egestas erat imperdiet. Rhoncus est pellentesque elit ullamcorper dignissim. Et ligula ullamcorper malesuada proin libero nunc consequat.

	Nunc mattis enim ut tellus elementum sagittis vitae et leo. Turpis tincidunt id aliquet risus feugiat in ante. Volutpat diam ut venenatis tellus in metus vulputate eu. Tincidunt praesent semper feugiat nibh. Sollicitudin aliquam ultrices sagittis orci a scelerisque purus. Ultrices vitae auctor eu augue ut. Nec dui nunc mattis enim ut tellus elementum sagittis. Fermentum et sollicitudin ac orci phasellus egestas tellus. Platea dictumst vestibulum rhoncus est pellentesque elit ullamcorper dignissim cras. Aliquam malesuada bibendum arcu vitae elementum curabitur vitae. Sodales neque sodales ut etiam sit amet nisl. Lectus quam id leo in vitae turpis. Lorem ipsum dolor sit amet consectetur adipiscing. Aliquam nulla facilisi cras fermentum odio eu feugiat. Integer eget aliquet nibh praesent tristique magna sit amet purus. Congue nisi vitae suscipit tellus mauris a diam maecenas sed. Vulputate eu scelerisque felis imperdiet proin. Posuere ac ut consequat semper viverra. Est sit amet facilisis magna etiam tempor.

	Eget sit amet tellus cras adipiscing enim eu turpis. Vulputate eu scelerisque felis imperdiet proin fermentum leo vel orci. Nisl purus in mollis nunc sed id semper risus. Quisque sagittis purus sit amet volutpat. Feugiat sed lectus vestibulum mattis ullamcorper velit sed ullamcorper. Pulvinar pellentesque habitant morbi tristique. Viverra aliquet eget sit amet tellus cras adipiscing. Blandit turpis cursus in hac habitasse platea dictumst quisque. Nisi est sit amet facilisis magna etiam. Vitae auctor eu augue ut lectus arcu. Iaculis urna id volutpat lacus laoreet.

	Tempus imperdiet nulla malesuada pellentesque elit eget gravida cum sociis. Lectus quam id leo in vitae turpis massa sed. In massa tempor nec feugiat. Sed blandit libero volutpat sed cras ornare arcu dui vivamus. Ut faucibus pulvinar elementum integer enim neque. Praesent semper feugiat nibh sed pulvinar proin gravida. Nunc congue nisi vitae suscipit. At auctor urna nunc id cursus metus aliquam. Nec feugiat nisl pretium fusce. Praesent elementum facilisis leo vel fringilla est ullamcorper eget nulla. Sem integer vitae justo eget magna fermentum. Eget mauris pharetra et ultrices. Aliquet porttitor lacus luctus accumsan tortor. Molestie a iaculis at erat pellentesque adipiscing commodo elit at. Libero enim sed faucibus turpis in eu. Vestibulum sed arcu non odio euismod. Sagittis purus sit amet volutpat consequat mauris nunc congue. Sollicitudin aliquam ultrices sagittis orci a. Quam elementum pulvinar etiam non quam lacus. In eu mi bibendum neque.

	Pellentesque massa placerat duis ultricies lacus. Commodo viverra maecenas accumsan lacus vel. Mi in nulla posuere sollicitudin. Varius vel pharetra vel turpis nunc eget lorem. Leo in vitae turpis massa. Amet consectetur adipiscing elit pellentesque habitant morbi tristique senectus. Amet porttitor eget dolor morbi non arcu risus. Vulputate dignissim suspendisse in est ante in nibh mauris cursus. Cras semper auctor neque vitae tempus quam pellentesque nec. Fringilla urna porttitor rhoncus dolor. Et egestas quis ipsum suspendisse ultrices gravida dictum fusce ut. Diam sollicitudin tempor id eu. Quis hendrerit dolor magna eget est lorem. Id volutpat lacus laoreet non curabitur gravida arcu ac tortor. Velit ut tortor pretium viverra suspendisse potenti nullam ac. Quis lectus nulla at volutpat diam ut venenatis.`)

	fn := filepath.Join(t.TempDir(), "encrypted")
	f, err := os.Create(fn)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := EncryptHeader(f, header, sk.PublicKey()); err != nil {
		t.Fatalf("EncryptHeader: %v", err)
	}
	w := EncryptFile(f, header)
	if _, err := io.Copy(w, bytes.NewBuffer(orig)); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}
	w.Close()

	if f, err = os.Open(fn); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	header2, err := DecryptHeader(f, sk)
	if err != nil {
		t.Fatalf("DecryptHeader: %v", err)
	}

	var decrypted bytes.Buffer
	reader := DecryptFile(f, header2)
	if _, err := io.Copy(&decrypted, reader); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}
	if want, got := orig, decrypted.Bytes(); bytes.Compare(want, got) != 0 {
		t.Errorf("Unexpected plaintext. Want %q, got %q", want, got)
	}

	if n, err := reader.Seek(6, io.SeekStart); n != 6 || err != nil {
		t.Fatalf("Seek: Want: 6, nil, got: %d %v", n, err)
	}
	buf := make([]byte, 5)
	if n, err := reader.Read(buf); n != 5 || err != nil {
		t.Fatalf("Read: Want: 5, nil, got: %d %v", n, err)
	}
	if want, got := "ipsum", string(buf); want != got {
		t.Errorf("Unexpected read. Want %q, got %q", want, got)
	}

	if n, err := reader.Seek(207, io.SeekStart); n != 207 || err != nil {
		t.Fatalf("Seek: Want: 207, nil, got: %d %v", n, err)
	}
	buf = make([]byte, 6)
	if n, err := reader.Read(buf); n != 6 || err != nil {
		t.Fatalf("Read: Want: 5, nil, got: %d %v", n, err)
	}
	if want, got := "tortor", string(buf); want != got {
		t.Errorf("Unexpected read. Want %q, got %q", want, got)
	}

	if n, err := reader.Seek(-10, io.SeekEnd); n != 4430 || err != nil {
		t.Fatalf("Seek: Want: 4430, nil, got: %d %v", n, err)
	}
	buf = make([]byte, 10)
	if n, err := reader.Read(buf); n != 10 || err != nil {
		t.Fatalf("Read: Want: 10, nil, got: %d %v", n, err)
	}
	if want, got := "venenatis.", string(buf); want != got {
		t.Errorf("Unexpected read. Want %q, got %q", want, got)
	}
}
