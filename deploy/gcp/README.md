# `deploy/gcp` — Cloud Run con Terraform

Módulo raíz único que despliega las tres aplicaciones en **Google Cloud Run**
(`us-central1`), con **Artifact Registry** para las imágenes y **Secret Manager**
para las credenciales. Provider `hashicorp/google ~> 8.0`, Terraform ≥ 1.16.

No se ejecuta a mano: lo llama [`scripts/deploy-gcp.sh`](../../scripts/deploy-gcp.sh)
a través de `make deploy-plan` / `make deploy`. La guía completa está en
el README de la raíz (sección «Despliegue»).

## Qué crea

| Fichero | Recursos |
|---|---|
| `apis.tf` | Habilita `run`, `artifactregistry`, `secretmanager`, `iam` y `cloudresourcemanager` (`disable_on_destroy = false`). |
| `artifact-registry.tf` | Repositorio Docker `proyectot` con política de limpieza: conserva las 2 versiones más recientes, borra las no etiquetadas y las de más de 30 días. |
| `secrets.tf` | `proyectot-jwt-secret` y `proyectot-auth-client-secret-sha256`, con acceso `secretAccessor` sólo para la cuenta de servicio que los necesita. |
| `iam.tf` | Tres cuentas de servicio (`qr-api-sa`, `stats-api-sa`, `web-sa`) **sin ningún rol a nivel de proyecto**, y `roles/run.invoker` para `allUsers` en los tres servicios. |
| `cloudrun.tf` | Tres `google_cloud_run_v2_service`: mín. 0 / máx. 3 instancias, 1 vCPU, 512 MiB (256 MiB el frontend), `cpu_idle = true`, puerto 8080, *startup probe* HTTP. |
| `outputs.tf` | Las tres URLs públicas, el registro de imágenes y las cuentas de servicio. |

## Por qué los tres servicios son públicos

El navegador llama **directamente** a `stats-api` (botón «recalcular en Node» y
badge de salud), así que no puede ser de *ingress* interno. La frontera de
seguridad es el JWT, no la red. Las tres cuentas de servicio son distintas y sin
permisos de proyecto: si una imagen se viera comprometida, no tendría nada que
alcanzar salvo el secreto que su propio servicio necesita.

## El ciclo CORS y la URL determinista

Cada servicio de Cloud Run responde en **dos** nombres: el determinista
(`https://<servicio>-<número de proyecto>.<región>.run.app`) y uno heredado con
un hash por proyecto, que es el que el proveedor expone como `.uri`. Los dos
sirven el mismo servicio; este módulo estandariza el determinista. Primero,
porque se calcula a partir de `data.google_project.this.number` —una entrada, no
una salida del grafo—, así que ambas APIs pueden llevar el origen del frontend en
`CORS_ALLOWED_ORIGINS` y el frontend las URLs de ambas APIs sin el ciclo
`qr-api → web.uri → qr-api.uri`. Segundo, porque es la **misma cadena** en la
lista CORS, en la config del navegador, en los outputs (`qr_api_url`,
`stats_api_url`, `web_url`) y en este README. El único `.uri` que queda es
`qr-api → stats-api`, donde además ordena la creación; los nombres con hash están
en el output `legacy_urls`, sólo informativo. Quien comprueba que todo encaja es
`deploy-gcp.sh`: antes del smoke test espera (`wait_for_url`) un `200` en
`<qr_api_url>/health/live`, `<stats_api_url>/health/live` y `<web_url>/config.js`,
y aborta si alguna no responde.

## Uso

```bash
cp terraform.tfvars.example terraform.tfvars   # ajusta project_id
make deploy-plan                                # terraform plan, sin cambios
make deploy                                     # build + push + apply + smoke test
./scripts/deploy-gcp.sh destroy                 # eliminar todo
```

Los secretos **nunca** se escriben en `terraform.tfvars`: el script los lee de
`.env` y los exporta como `TF_VAR_jwt_secret` y
`TF_VAR_auth_client_secret_sha256`.

## Estado

Estado **local** (`terraform.tfstate`, en `.gitignore`). Es deliberado para una
prueba técnica: un backend remoto en GCS necesitaría un bucket que a su vez
habría que crear en algún sitio. Para uso real, la migración es añadir un bloque
`backend "gcs"` en `versions.tf` y `terraform init -migrate-state`.
