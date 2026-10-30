-- 0008: alianzas comerciales proveedor ↔ tienda.
--
-- Un proveedor solo debe operar con tiendas con las que tiene un acuerdo
-- comercial. La relación es muchos-a-muchos: un proveedor puede surtir varias
-- tiendas y una tienda puede tener varios proveedores.
--
-- La distancia no se persiste aquí: se calcula con PostGIS sobre
-- app.stores.location al consultar el catálogo (ver modelo_datos.md §2.3).
-- El rol PROVIDER del proveedor no se puede imponer con un CHECK (requiere
-- consultar app.users); deberá validarse en el caso de uso que cree el acuerdo.

CREATE TYPE app.provider_store_status AS ENUM ('ACTIVE', 'INACTIVE');

CREATE TABLE app.provider_store_agreements (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES app.users(id) ON DELETE CASCADE,
    store_id    UUID NOT NULL REFERENCES app.stores(id) ON DELETE CASCADE,
    status      app.provider_store_status NOT NULL DEFAULT 'ACTIVE',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Un solo acuerdo vigente por par proveedor-tienda; el estado permite
    -- suspenderlo sin borrar la relación.
    CONSTRAINT provider_store_agreements_unique UNIQUE (provider_id, store_id)
);

-- Catálogo del proveedor (solo acuerdos activos) y consulta inversa por tienda.
CREATE INDEX idx_provider_store_agreements_provider
    ON app.provider_store_agreements (provider_id, status);
CREATE INDEX idx_provider_store_agreements_store
    ON app.provider_store_agreements (store_id);

-- Permite filtrar por radio en metros con ST_DWithin(...::geography) usando
-- índice GiST, sin convertir la columna de stores a geography.
CREATE INDEX idx_stores_location_geography
    ON app.stores USING GIST ((location::geography));

COMMENT ON TABLE app.provider_store_agreements IS
    'Alianzas comerciales proveedor-tienda: define qué tiendas puede atender cada proveedor';
COMMENT ON COLUMN app.provider_store_agreements.status IS
    'ACTIVE = acuerdo vigente y visible en el catálogo; INACTIVE = suspendido';
