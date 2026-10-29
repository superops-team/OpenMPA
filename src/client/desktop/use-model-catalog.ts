import { useEffect, useState } from 'react';
import type { ConnectionVM, DesktopServices } from './models';
import type { RuntimeModelCatalog } from './model-catalog';

type Catalog = RuntimeModelCatalog & { expires: number };
const catalogs = new WeakMap<DesktopServices, Map<string, Promise<Catalog>>>();
const ttl = 12 * 60 * 60 * 1000;

// One catalog request per account/connection, shared by settings and composer.
// Rejections are not cached, and a new connection never sees the previous list.
function load(
  services: DesktopServices,
  key: string,
  epoch: number,
  id: string,
): Promise<Catalog> {
  let cache = catalogs.get(services);
  if (!cache) catalogs.set(services, (cache = new Map()));
  const cached = cache.get(key);
  if (cached)
    return cached.then((value) => {
      if (value.expires > Date.now()) return value;
      if (cache.get(key) === cached) cache.delete(key);
      return load(services, key, epoch, id);
    });
  const pending = Promise.resolve().then(async () => {
    if (services.sessions.models) {
      const catalog = await services.sessions.models(epoch);
      return { ...catalog, expires: Date.now() + ttl };
    }
    // Legacy adapter clients use the same native eligibility implementation.
    if (!services.sessions.modelCatalog)
      throw new Error('modelCatalogUnavailable');
    const models = await services.sessions.modelCatalog(epoch, id);
    return {
      models,
      defaultModel: '',
      supported: models.length > 0,
      expires: Date.now() + ttl,
    };
  });
  cache.set(key, pending);
  void pending.catch(() => {
    if (cache.get(key) === pending) cache.delete(key);
  });
  return pending;
}

export function useModelCatalog(
  services: DesktopServices,
  connection: ConnectionVM,
  id = '',
) {
  const key = `${connection.scope}:${connection.epoch}`;
  const [attempt, setAttempt] = useState(0);
  const [result, setResult] = useState<{
    key: string;
    expires: number;
    models: RuntimeModelCatalog['models'];
    defaultModel: string;
    supported: boolean;
    loading: boolean;
    failed: boolean;
  }>({
    key,
    expires: 0,
    models: [],
    defaultModel: '',
    supported: false,
    loading: true,
    failed: false,
  });
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    setResult({
      key,
      expires: 0,
      models: [],
      defaultModel: '',
      supported: false,
      loading: true,
      failed: false,
    });
    void load(services, key, connection.epoch, id).then(
      ({ models, defaultModel, supported, expires }) => {
        if (active) {
          setResult({
            key,
            expires,
            models,
            defaultModel,
            supported,
            loading: false,
            failed: false,
          });
          timer = setTimeout(
            () => {
              if (active) setAttempt((value) => value + 1);
            },
            Math.max(1, expires - Date.now()),
          );
        }
      },
      () => {
        if (active)
          setResult({
            key,
            expires: 0,
            models: [],
            defaultModel: '',
            supported: false,
            loading: false,
            failed: true,
          });
      },
    );
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [services, key, connection.epoch, id, attempt]);
  return {
    ...(result.key === key &&
    (result.expires > Date.now() || result.failed || result.loading)
      ? result
      : {
          models: [],
          defaultModel: '',
          supported: false,
          loading: true,
          failed: false,
        }),
    retry: () => setAttempt((value) => value + 1),
  };
}
