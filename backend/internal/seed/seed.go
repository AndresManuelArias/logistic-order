// Package seed carga datos de demostración idempotentes para desarrollo (F2).
//
// Reglas:
//   - Nunca sobrescribe datos existentes: si el proveedor seed ya existe, no
//     hace nada (ejecutar `make seed` es siempre seguro).
//   - Solo crea el rol PROVIDER (los STORE/DRIVER pueden auto-registrarse).
//   - Las órdenes usan coordenadas reales de Bogotá para que PostGIS, la
//     distancia esférica y el Continuous Aggregate de paradas tengan datos.
package seed

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authinfrastructure "tiendas-track/backend/internal/auth/infrastructure"
)

// Resultado del seed para informar al operador.
type Result struct {
	Skipped  bool
	Users    int
	Stores   int
	Vehicles int
	Drivers  int
	Orders   int
}

// credentials son las credenciales seed (solo desarrollo; ver README).
type account struct {
	Email    string
	Password string
	FullName string
}

var (
	providerAcc = account{Email: "proveedor@tiendas.dev", Password: "Proveedor123", FullName: "Logística Andina"}
	storeAccs   = []account{
		{Email: "tienda1@tiendas.dev", Password: "Tienda12345", FullName: "Tienda La Esquina"},
		{Email: "tienda2@tiendas.dev", Password: "Tienda12345", FullName: "Tienda El Mercado"},
	}
	driverAccs = []account{
		{Email: "conductor1@tiendas.dev", Password: "Conductor123", FullName: "Carlos Ramírez"},
		{Email: "conductor2@tiendas.dev", Password: "Conductor123", FullName: "Lucía Fernández"},
		{Email: "conductor3@tiendas.dev", Password: "Conductor123", FullName: "Andrés Gómez"},
	}
)

// Run crea los datos demo. Es idempotente.
func Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (*Result, error) {
	res := &Result{}

	// Si el proveedor seed ya existe, el seed ya corrió: salir sin tocar nada.
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app.users WHERE email = $1)`, providerAcc.Email).Scan(&exists); err != nil {
		return nil, fmt.Errorf("seed: verificando datos previos: %w", err)
	}
	if exists {
		log.Info("seed: datos ya presentes, nada que hacer")
		res.Skipped = true
		return res, nil
	}

	hasher := authinfrastructure.NewPasswordHasher(10) // cost 10: suficiente para demo

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("seed: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // el commit explícito abajo es el camino feliz

	// --- Proveedor (rol que nadie puede auto-registrarse) --------------------
	providerID, err := insertUser(ctx, tx, providerAcc, "PROVIDER", hasher)
	if err != nil {
		return nil, err
	}
	res.Users++

	// --- Vehículos del proveedor --------------------------------------------
	plates := []string{"ABC123", "DEF456", "GHI789"}
	vehicleIDs := make([]string, 0, len(plates))
	for _, plate := range plates {
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO app.vehicles (plate, provider_id, status)
			VALUES ($1, $2::uuid, 'ACTIVE') RETURNING id::text`, plate, providerID).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("seed: vehículo %s: %w", plate, err)
		}
		vehicleIDs = append(vehicleIDs, id)
		res.Vehicles++
	}

	// --- Tiendas (owner = usuario STORE) -------------------------------------
	// Coordenadas: Chapinero (tienda 1) y Teusaquillo (tienda 2), Bogotá.
	storeCoords := [][2]float64{{-74.0632, 4.6550}, {-74.0830, 4.6270}}
	storeIDs := make([]string, 0, len(storeAccs))
	for i, acc := range storeAccs {
		ownerID, err := insertUser(ctx, tx, acc, "STORE", hasher)
		if err != nil {
			return nil, err
		}
		res.Users++
		lon, lat := storeCoords[i][0], storeCoords[i][1]
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO app.stores (name, owner_user_id, address, location)
			VALUES ($1, $2::uuid, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326))
			RETURNING id::text`, acc.FullName, ownerID, "Bogotá D.C.", lon, lat).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("seed: tienda %s: %w", acc.FullName, err)
		}
		storeIDs = append(storeIDs, id)
		res.Stores++
	}

	// --- Conductores (usuario + vehículo asignado) ---------------------------
	driverIDs := make([]string, 0, len(driverAccs))
	for i, acc := range driverAccs {
		userID, err := insertUser(ctx, tx, acc, "DRIVER", hasher)
		if err != nil {
			return nil, err
		}
		res.Users++
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO app.drivers (user_id, vehicle_id, license_number)
			VALUES ($1::uuid, $2::uuid, $3) RETURNING id::text`,
			userID, vehicleIDs[i%len(vehicleIDs)], fmt.Sprintf("LIC-%06d", 100000+i)).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("seed: conductor %s: %w", acc.FullName, err)
		}
		driverIDs = append(driverIDs, id)
		res.Drivers++
	}

	// --- Órdenes de demostración ---------------------------------------------
	// Estado de origen/destino: par de coordenadas por orden (Bogotá).
	type demoOrder struct {
		ref       string
		storeIdx  int
		status    string
		assignIdx int // -1 = sin asignar
		origin    [2]float64
		dest      [2]float64
		addr      string
	}
	orders := []demoOrder{
		{"PED-1001", 0, "CREATED", -1, [2]float64{-74.0632, 4.6550}, [2]float64{-74.0561, 4.6578}, "Cra 7 # 72-40"},
		{"PED-1002", 0, "IN_TRANSIT", 0, [2]float64{-74.0632, 4.6550}, [2]float64{-74.0721, 4.7108}, "Cl 93 # 13-45"},
		{"PED-1003", 0, "DELIVERED", 0, [2]float64{-74.0632, 4.6550}, [2]float64{-74.0480, 4.6690}, "Av 6 # 118-28"},
		{"PED-1004", 0, "CANCELLED", -1, [2]float64{-74.0632, 4.6550}, [2]float64{-74.1000, 4.6200}, "Av Suba # 55-10"},
		{"PED-1005", 0, "CREATED", -1, [2]float64{-74.0632, 4.6550}, [2]float64{-74.0612, 4.6480}, "Cl 39 # 7-50"},
		{"PED-1006", 1, "CREATED", -1, [2]float64{-74.0830, 4.6270}, [2]float64{-74.0900, 4.6400}, "Cl 26 # 68D-50"},
		{"PED-1007", 1, "IN_TRANSIT", 1, [2]float64{-74.0830, 4.6270}, [2]float64{-74.0700, 4.6100}, "Av Boyacá # 74-21"},
		{"PED-1008", 1, "DELIVERED", 1, [2]float64{-74.0830, 4.6270}, [2]float64{-74.0550, 4.6300}, "Cr 30 # 18-05"},
		{"PED-1009", 1, "IN_TRANSIT", 2, [2]float64{-74.0830, 4.6270}, [2]float64{-74.0750, 4.6500}, "Av NQS # 41-14"},
		{"PED-1010", 0, "CREATED", -1, [2]float64{-74.0632, 4.6550}, [2]float64{-74.1170, 4.6480}, "Av Cl 63 # 117-71"},
	}
	for i, o := range orders {
		vehicle, driver := any(nil), any(nil)
		if o.assignIdx >= 0 {
			vehicle = vehicleIDs[o.assignIdx%len(vehicleIDs)]
			driver = driverIDs[o.assignIdx%len(driverIDs)]
		}
		placedAt := time.Now().Add(-time.Duration(len(orders)-i) * time.Hour)
		var pickedUp, delivered any
		if o.status == "IN_TRANSIT" || o.status == "DELIVERED" {
			pickedUp = placedAt.Add(30 * time.Minute)
		}
		if o.status == "DELIVERED" {
			delivered = placedAt.Add(2 * time.Hour)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO app.orders (external_ref, store_id, status, assigned_vehicle_id, assigned_driver_id,
			                        origin, destination, destination_address, distance_meters,
			                        placed_at, picked_up_at, delivered_at)
			VALUES ($1, $2::uuid, $3::app.order_status, $4::uuid, $5::uuid,
			        ST_SetSRID(ST_MakePoint($6, $7), 4326),
			        ST_SetSRID(ST_MakePoint($8, $9), 4326),
			        $10, ST_DistanceSphere(
			              ST_SetSRID(ST_MakePoint($6, $7), 4326),
			              ST_SetSRID(ST_MakePoint($8, $9), 4326)),
			        $11, $12, $13)`,
			o.ref, storeIDs[o.storeIdx], o.status, vehicle, driver,
			o.origin[0], o.origin[1], o.dest[0], o.dest[1],
			o.addr, placedAt, pickedUp, delivered)
		if err != nil {
			return nil, fmt.Errorf("seed: orden %s: %w", o.ref, err)
		}
		res.Orders++

		// Evento inicial en el historial (append-only en BD).
		if _, err := tx.Exec(ctx, `
			INSERT INTO app.order_events (order_id, status, occurred_at, payload)
			SELECT id, $2::app.order_status, $3, '{"source":"seed"}'::jsonb
			FROM app.orders WHERE external_ref = $1`, o.ref, o.status, placedAt); err != nil {
			return nil, fmt.Errorf("seed: evento de %s: %w", o.ref, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("seed: commit: %w", err)
	}
	log.Info("seed completado",
		"users", res.Users, "stores", res.Stores,
		"vehicles", res.Vehicles, "drivers", res.Drivers, "orders", res.Orders)
	return res, nil
}

// insertUser crea un usuario con hash bcrypt y devuelve su ID (rol seed).
func insertUser(ctx context.Context, tx pgx.Tx, acc account, role string,
	hasher *authinfrastructure.PasswordHasher) (string, error) {
	hash, err := hasher.Hash(acc.Password)
	if err != nil {
		return "", fmt.Errorf("seed: hash de %s: %w", acc.Email, err)
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO app.users (email, password_hash, full_name, role)
		VALUES ($1, $2, $3, $4::app.role_enum)
		RETURNING id::text`, acc.Email, hash, acc.FullName, role).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("seed: usuario %s: %w", acc.Email, err)
	}
	return id, nil
}

// HashPassword expone el hash para pruebas.
func HashPassword(hasher *authinfrastructure.PasswordHasher, plain string) (string, error) {
	return hasher.Hash(strings.TrimSpace(plain))
}
