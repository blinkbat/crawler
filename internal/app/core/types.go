package core

type GameMap struct {
	Width  int
	Height int
	Rows   []string
}

type Player struct {
	TileX     int
	TileZ     int
	Facing    int
	X         float32
	Z         float32
	Yaw       float32
	LookYaw   float32
	LookPitch float32
	Anim      Animation
}

type Animation struct {
	Kind     int
	Elapsed  float32
	Duration float32
	FromX    float32
	FromZ    float32
	ToX      float32
	ToZ      float32
	FromYaw  float32
	ToYaw    float32
}

type GameState struct {
	Map       GameMap
	Player    Player
	Party     []PartyMember
	Enemies   []Enemy
	Battle    Battle
	MenuOpen  bool
	MenuIndex int
	Quit      bool
}

type PartyMember struct {
	Class PartyClass
	Name  string
	HP    int
	MaxHP int
	MP    int
	MaxMP int

	Attack      int
	AttackBump  float32
	DamageFlash float32
}

type Enemy struct {
	Kind        EnemyKind
	TileX       int
	TileZ       int
	HP          int
	MaxHP       int
	Alive       bool
	Name        string
	MonsterType string
	Item        string

	AttackBump  float32
	DamageFlash float32
	DeathFade   float32
	BurnTurns   int
}

type Battle struct {
	EnemyIndex   int
	EnemyGroup   []int
	CurrentParty int
	ActionMode   int
	MenuIndex    int
	PendingSkill int
	PartyTarget  int
	Phase        int
	Timer        float32
	Splash       float32
	Message      string
	Log          []string
}
