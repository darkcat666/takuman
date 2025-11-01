package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"log"
	"math/rand/v2"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	raudio "github.com/hajimehoshi/ebiten/v2/examples/resources/audio"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	resources "github.com/hajimehoshi/ebiten/v2/examples/resources/images/flappy"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

var flagCRT = flag.Bool("crt", false, "enable the CRT effect")

//go:embed crt.go
var crtGo []byte

//ebitengine:shaderfile crt.go

// 発射された弾の構造体
type Bullet struct {
	useFlag   bool
	liveFlag  bool
	speedFlag bool
	speed     float32
	image     *ebiten.Image
	posX      float32
	posY      float32
	shotPosX  float64
	shotPosY  float64
}

// Bullet構造体のインスタンスを生成
var bullet [maxBulletCount]Bullet

func floorDiv(x, y int) int {
	d := x / y
	if d*y == x || x >= 0 {
		return d
	}
	return d - 1
}

func floorMod(x, y int) int {
	return x - floorDiv(x, y)*y
}

const (
	screenWidth      = 640
	screenHeight     = 480
	tileSize         = 32
	titleFontSize    = fontSize * 1.5
	fontSize         = 24
	smallFontSize    = fontSize / 2
	pipeWidth        = tileSize * 2
	pipeStartOffsetX = 8
	pipeIntervalX    = 8
	pipeGapY         = 5
	maxBulletCount   = 100
	bulletSpeed      = 13
)

var (
	// 弾画像
	tmpBulletImage image.Image
	bulletFile     *ebiten.Image
	// 弾存在フラグ
	bulletCount int

	tmpGopherImage   image.Image
	gopherImage      *ebiten.Image
	tilesImage       *ebiten.Image
	arcadeFaceSource *text.GoTextFaceSource
)

func init() {
	// 弾画像の取得
	tmpBulletImage = readEbitenImage("./resources_666/fire_square_bullet.jpg")
	bulletFile = ebiten.NewImageFromImage(tmpBulletImage)

	tmpGopherImage = readEbitenImage("./resources_666/inu_man.png")
	gopherImage = ebiten.NewImageFromImage(tmpGopherImage)
	// img, _, err := image.Decode(bytes.NewReader(resources.Gopher_png))
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// gopherImage = ebiten.NewImageFromImage(img)

	img, _, err := image.Decode(bytes.NewReader(resources.Tiles_png))
	if err != nil {
		log.Fatal(err)
	}
	tilesImage = ebiten.NewImageFromImage(img)
}

func init() {
	s, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.PressStart2P_ttf))
	if err != nil {
		log.Fatal(err)
	}
	arcadeFaceSource = s
}

// Ebiten画像を読み込む
func readEbitenImage(filePath string) image.Image {
	// ファイルを開く
	file, err := os.Open(filePath) // 画像ファイル名
	if err != nil {
		panic(err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		panic(err)
	}
	return img
}

type Mode int

const (
	ModeTitle Mode = iota
	ModeGame
	ModeGameOver
)

type Game struct {
	mode Mode

	// The gopher's position
	x16  int
	y16  int
	vy16 int

	// Camera
	cameraX int
	cameraY int

	// Pipes
	pipeTileYs []int

	gameoverCount int

	touchIDs   []ebiten.TouchID
	gamepadIDs []ebiten.GamepadID

	audioContext *audio.Context
	jumpPlayer   *audio.Player
	hitPlayer    *audio.Player
}

func NewGame(crt bool) ebiten.Game {
	g := &Game{}
	g.init()
	if crt {
		return &GameWithCRTEffect{Game: g}
	}
	return g
}

func (g *Game) init() {
	// スクリーンサイズを取得
	Width, Height := 1080, 960

	g.x16 = Width / 2
	g.y16 = Height
	g.cameraX = -240
	g.cameraY = 0
	g.pipeTileYs = make([]int, 256)
	for i := range g.pipeTileYs {
		g.pipeTileYs[i] = rand.IntN(6) + 2
	}

	if g.audioContext == nil {
		g.audioContext = audio.NewContext(48000)
	}

	jumpD, err := vorbis.DecodeF32(bytes.NewReader(raudio.Jump_ogg))
	if err != nil {
		log.Fatal(err)
	}
	g.jumpPlayer, err = g.audioContext.NewPlayerF32(jumpD)
	if err != nil {
		log.Fatal(err)
	}

	jabD, err := wav.DecodeF32(bytes.NewReader(raudio.Jab_wav))
	if err != nil {
		log.Fatal(err)
	}
	g.hitPlayer, err = g.audioContext.NewPlayerF32(jabD)
	if err != nil {
		log.Fatal(err)
	}
}

func (g *Game) isKeyJustPressed() bool {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		return true
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return true
	}
	g.touchIDs = inpututil.AppendJustPressedTouchIDs(g.touchIDs[:0])
	if len(g.touchIDs) > 0 {
		return true
	}
	g.gamepadIDs = ebiten.AppendGamepadIDs(g.gamepadIDs[:0])
	for _, g := range g.gamepadIDs {
		if ebiten.IsStandardGamepadLayoutAvailable(g) {
			if inpututil.IsStandardGamepadButtonJustPressed(g, ebiten.StandardGamepadButtonRightBottom) {
				return true
			}
			if inpututil.IsStandardGamepadButtonJustPressed(g, ebiten.StandardGamepadButtonRightRight) {
				return true
			}
		} else {
			// The button 0/1 might not be A/B buttons.
			if inpututil.IsGamepadButtonJustPressed(g, ebiten.GamepadButton0) {
				return true
			}
			if inpututil.IsGamepadButtonJustPressed(g, ebiten.GamepadButton1) {
				return true
			}
		}
	}
	return false
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) Update() error {
	switch g.mode {
	case ModeTitle:
		if g.isKeyJustPressed() {
			g.mode = ModeGame
		}
	case ModeGame:
		g.x16 += 32
		g.cameraX += 2
		if g.isKeyJustPressed() {
			g.vy16 = -96
			if err := g.jumpPlayer.Rewind(); err != nil {
				return err
			}
			g.jumpPlayer.Play()
		}
		g.y16 += g.vy16

		// Gravity
		g.vy16 += 4
		if g.vy16 > 96 {
			g.vy16 = 96
		}

		// 接地判定
		// （イミフな判定・・・）
		if g.y16 > 6800 {
			g.y16 = 6800
			g.vy16 = 0
		}

		// if g.hit() {
		// 	if err := g.hitPlayer.Rewind(); err != nil {
		// 		return err
		// 	}
		// 	g.hitPlayer.Play()
		// 	g.mode = ModeGameOver
		// 	g.gameoverCount = 30
		// }
	case ModeGameOver:
		if g.gameoverCount > 0 {
			g.gameoverCount--
		}
		if g.gameoverCount == 0 && g.isKeyJustPressed() {
			g.init()
			g.mode = ModeTitle
		}
	}
	return nil
}

// 弾を管理する関数
func (g *Game) manageBullets() {
	// 「E」キー押下で弾を発射（最大弾数を超過していない場合に発動！）
	// 中の処理でbulletCountをインクリメントしているから条件に+1を追加した
	if inpututil.IsKeyJustPressed(ebiten.KeyE) && bulletCount+1 < maxBulletCount {
		// 発射処理に先んじて、弾カウントを１増やす
		bulletCount++
		// 弾を移動させるスピードを初期化する
		for i := 0; i < bulletCount; i++ {
			bullet[bulletCount].speed = 0
		}
		// 弾のY軸の発射位置を保存
		bullet[bulletCount].shotPosY = float64(g.y16)
		// 対象の弾を発射したフラグをtrueにする
		bullet[bulletCount].liveFlag = true
		bullet[bulletCount].useFlag = true
	} else if inpututil.IsKeyJustPressed(ebiten.KeyE) && bulletCount+1 == maxBulletCount {
		// リロード処理
		bulletCount = 0
		// 発射処理に先んじて、弾カウントを１増やす
		bulletCount++
		// 弾を移動させるスピードを初期化する
		for i := 0; i < bulletCount; i++ {
			bullet[bulletCount].speed = 0
		}
		// 弾のY軸の発射位置を保存
		bullet[bulletCount].shotPosY = float64(g.y16)
		// 対象の弾を発射したフラグをtrueにする
		bullet[bulletCount].liveFlag = true
		bullet[bulletCount].useFlag = true
	}
}

// 弾をクリーンアップする関数
func clearBullets(bulletData *Bullet) {
	// スクリーンサイズをあらかじめ取得
	screenWidth, _ := ebiten.Monitor().Size()

	for i := 0; i < bulletCount; i++ {
		// 無効な弾を除外して計算する
		if !bulletData.liveFlag {
			continue
		}
		// 弾が画面外に出た場合
		if (int(bulletData.posX) + gopherImage.Bounds().Dx()) > screenWidth {
			// すべての弾のフラグをクリアする関数
			clearAllBulletFlag(bulletData)
		}
	}
}

// すべての弾のフラグをクリアする関数
func clearAllBulletFlag(targetBullet *Bullet) {
	for i := 0; i < len(bullet); i++ {
		// 管理するフラグ変数もすべてクリアする
		targetBullet.liveFlag = false
		targetBullet.useFlag = false
		targetBullet.speedFlag = false
		// 弾のスピードもクリアする
		targetBullet.speed = 0
		// 弾の画像もクリアする
		targetBullet.image = nil
		// 弾の位置もクリアする
		targetBullet.posX = float32(gopherImage.Bounds().Dx())
		targetBullet.posY = float32(gopherImage.Bounds().Dy())
		// 弾の発射位置もクリアする（？？？）
		targetBullet.shotPosX = 0
		targetBullet.shotPosY = 0

	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x80, 0xa0, 0xc0, 0xff})
	g.drawTiles(screen)
	if g.mode != ModeTitle {
		g.drawGopher(screen)
	}

	// 弾を管理する関数（副作用として弾のY軸オフセットを取得）
	g.manageBullets()

	for i := 0; i < bulletCount; i++ {
		// 対象の弾を発射したフラグがtrueの時に
		if bullet[i].useFlag {
			// 弾を発射
			g.drawBullet(screen, &bullet[i])
			// 使い終わった弾をクリーンアップする関数
			clearBullets(&bullet[i])
		}
	}

	var titleTexts string
	var texts string
	switch g.mode {
	case ModeTitle:
		titleTexts = "TAKUMAN"
		texts = "\n\n\n\n\n\nPRESS SPACE KEY\n\nOR A/B BUTTON\n\nOR TOUCH SCREEN"
	case ModeGameOver:
		texts = "\nYOU DIED!"
	}

	op := &text.DrawOptions{}
	op.GeoM.Translate(screenWidth/2, 3*titleFontSize)
	op.ColorScale.ScaleWithColor(color.White)
	op.LineSpacing = titleFontSize
	op.PrimaryAlign = text.AlignCenter
	text.Draw(screen, titleTexts, &text.GoTextFace{
		Source: arcadeFaceSource,
		Size:   titleFontSize,
	}, op)

	op = &text.DrawOptions{}
	op.GeoM.Translate(screenWidth/2, 3*titleFontSize)
	op.ColorScale.ScaleWithColor(color.White)
	op.LineSpacing = fontSize
	op.PrimaryAlign = text.AlignCenter
	text.Draw(screen, texts, &text.GoTextFace{
		Source: arcadeFaceSource,
		Size:   fontSize,
	}, op)

	if g.mode == ModeTitle {
		const msg = "Go Gopher by Renee French is\nlicenced under CC BY 3.0."

		op := &text.DrawOptions{}
		op.GeoM.Translate(screenWidth/2, screenHeight-smallFontSize/2)
		op.ColorScale.ScaleWithColor(color.White)
		op.LineSpacing = smallFontSize
		op.PrimaryAlign = text.AlignCenter
		op.SecondaryAlign = text.AlignEnd
		text.Draw(screen, msg, &text.GoTextFace{
			Source: arcadeFaceSource,
			Size:   smallFontSize,
		}, op)
	}

	op = &text.DrawOptions{}
	op.GeoM.Translate(screenWidth, 0)
	op.ColorScale.ScaleWithColor(color.White)
	op.LineSpacing = fontSize
	op.PrimaryAlign = text.AlignEnd
	text.Draw(screen, fmt.Sprintf("%04d", g.score()), &text.GoTextFace{
		Source: arcadeFaceSource,
		Size:   fontSize,
	}, op)

	ebitenutil.DebugPrint(screen, fmt.Sprintf("TPS: %0.2f", ebiten.ActualTPS()))
}

func (g *Game) pipeAt(tileX int) (tileY int, ok bool) {
	if (tileX - pipeStartOffsetX) <= 0 {
		return 0, false
	}
	if floorMod(tileX-pipeStartOffsetX, pipeIntervalX) != 0 {
		return 0, false
	}
	idx := floorDiv(tileX-pipeStartOffsetX, pipeIntervalX)
	return g.pipeTileYs[idx%len(g.pipeTileYs)], true
}

func (g *Game) score() int {
	x := floorDiv(g.x16, 16) / tileSize
	if (x - pipeStartOffsetX) <= 0 {
		return 0
	}
	return floorDiv(x-pipeStartOffsetX, pipeIntervalX)
}

func (g *Game) hit() bool {
	if g.mode != ModeGame {
		return false
	}
	const (
		gopherWidth  = 30
		gopherHeight = 60
	)
	w, h := gopherImage.Bounds().Dx(), gopherImage.Bounds().Dy()
	x0 := floorDiv(g.x16, 16) + (w-gopherWidth)/2
	y0 := floorDiv(g.y16, 16) + (h-gopherHeight)/2
	x1 := x0 + gopherWidth
	y1 := y0 + gopherHeight
	if y0 < -tileSize*4 {
		return true
	}
	if y1 >= screenHeight-tileSize {
		return true
	}
	xMin := floorDiv(x0-pipeWidth, tileSize)
	xMax := floorDiv(x0+gopherWidth, tileSize)
	for x := xMin; x <= xMax; x++ {
		y, ok := g.pipeAt(x)
		if !ok {
			continue
		}
		if x0 >= x*tileSize+pipeWidth {
			continue
		}
		if x1 < x*tileSize {
			continue
		}
		if y0 < y*tileSize {
			return true
		}
		if y1 >= (y+pipeGapY)*tileSize {
			return true
		}
	}
	return false
}

func (g *Game) drawTiles(screen *ebiten.Image) {
	const (
		nx           = screenWidth / tileSize
		ny           = screenHeight / tileSize
		pipeTileSrcX = 128
		pipeTileSrcY = 192
	)

	op := &ebiten.DrawImageOptions{}
	for i := -2; i < nx+1; i++ {
		// ground
		op.GeoM.Reset()
		op.GeoM.Translate(float64(i*tileSize-floorMod(g.cameraX, tileSize)),
			float64((ny-1)*tileSize-floorMod(g.cameraY, tileSize)))
		screen.DrawImage(tilesImage.SubImage(image.Rect(0, 0, tileSize, tileSize)).(*ebiten.Image), op)

		// pipe
		if tileY, ok := g.pipeAt(floorDiv(g.cameraX, tileSize) + i); ok {
			for j := 0; j < tileY; j++ {
				op.GeoM.Reset()
				op.GeoM.Scale(1, -1)
				op.GeoM.Translate(float64(i*tileSize-floorMod(g.cameraX, tileSize)),
					float64(j*tileSize-floorMod(g.cameraY, tileSize)))
				op.GeoM.Translate(0, tileSize)
				var r image.Rectangle
				if j == tileY-1 {
					r = image.Rect(pipeTileSrcX, pipeTileSrcY, pipeTileSrcX+tileSize*2, pipeTileSrcY+tileSize)
				} else {
					r = image.Rect(pipeTileSrcX, pipeTileSrcY+tileSize, pipeTileSrcX+tileSize*2, pipeTileSrcY+tileSize*2)
				}
				screen.DrawImage(tilesImage.SubImage(r).(*ebiten.Image), op)
			}
			for j := tileY + pipeGapY; j < screenHeight/tileSize-1; j++ {
				op.GeoM.Reset()
				op.GeoM.Translate(float64(i*tileSize-floorMod(g.cameraX, tileSize)),
					float64(j*tileSize-floorMod(g.cameraY, tileSize)))
				var r image.Rectangle
				if j == tileY+pipeGapY {
					r = image.Rect(pipeTileSrcX, pipeTileSrcY, pipeTileSrcX+pipeWidth, pipeTileSrcY+tileSize)
				} else {
					r = image.Rect(pipeTileSrcX, pipeTileSrcY+tileSize, pipeTileSrcX+pipeWidth, pipeTileSrcY+tileSize+tileSize)
				}
				screen.DrawImage(tilesImage.SubImage(r).(*ebiten.Image), op)
			}
		}
	}
}

// 弾の描画メソッド
// 弾を管理する変数を添え時にする
func (g *Game) drawBullet(screen *ebiten.Image, bullet *Bullet) {
	bullet.image = bulletFile
	if !bullet.speedFlag && bullet.liveFlag {
		// 弾を移動させるスピードを定義する処理
		bullet.speed += float32(bulletSpeed)
	}
	// 弾の位置を補足する処理
	bullet.posX += float32(bulletSpeed)

	op := &ebiten.DrawImageOptions{}
	// w, h := gopherImage.Bounds().Dx(), gopherImage.Bounds().Dy()
	// op.GeoM.Translate(-float64(w)/2.0, -float64(h)/2.0)
	// op.GeoM.Rotate(float64(g.vy16) / 96.0 * math.Pi / 6)
	// op.GeoM.Translate(float64(w)/2.0, float64(h)/2.0)
	// 弾のY座標の位置は上下にぶれないため、発射した位置で固定

	op.GeoM.Translate(float64(g.x16/16.0)+float64(g.cameraX)+bulletSpeed, bullet.shotPosY/16.0)
	op.Filter = ebiten.FilterLinear
	screen.DrawImage(bullet.image, op)
}

func (g *Game) drawGopher(screen *ebiten.Image) {
	op := &ebiten.DrawImageOptions{}
	// w, h := gopherImage.Bounds().Dx(), gopherImage.Bounds().Dy()
	// op.GeoM.Translate(-float64(w)/2.0, -float64(h)/2.0)
	// op.GeoM.Rotate(float64(g.vy16) / 96.0 * math.Pi / 6)
	// op.GeoM.Translate(float64(w)/2.0, float64(h)/2.0)
	op.GeoM.Translate(float64(g.x16/16.0)-float64(g.cameraX), float64(g.y16/16.0)-float64(g.cameraY))
	op.Filter = ebiten.FilterLinear
	screen.DrawImage(gopherImage, op)
}

type GameWithCRTEffect struct {
	ebiten.Game

	crtShader *ebiten.Shader
}

func (g *GameWithCRTEffect) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, geoM ebiten.GeoM) {
	if g.crtShader == nil {
		s, err := ebiten.NewShader(crtGo)
		if err != nil {
			panic(fmt.Sprintf("flappy: failed to compiled the CRT shader: %v", err))
		}
		g.crtShader = s
	}

	os := offscreen.Bounds().Size()

	op := &ebiten.DrawRectShaderOptions{}
	op.Images[0] = offscreen
	op.GeoM = geoM
	screen.DrawRectShader(os.X, os.Y, g.crtShader, op)
}

func main() {
	flag.Parse()
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("takuman")
	if err := ebiten.RunGame(NewGame(*flagCRT)); err != nil {
		panic(err)
	}
}
