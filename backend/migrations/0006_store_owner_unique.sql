-- 0006: una tienda por dueño (RF-02) protegida a nivel de datos.
-- La regla ya vivía solo en la aplicación; ahora la base de datos la garantiza
-- y convierte la carrera entre dos altas simultáneas en un 23505 manejable.
CREATE UNIQUE INDEX idx_stores_owner_unique ON app.stores (owner_user_id);
