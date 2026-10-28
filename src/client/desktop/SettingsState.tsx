import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react';

import type {
  ConnectionVM,
  DesktopServices,
  ImagesVM,
  PlatformVM,
  RegionVM,
  RuntimeVM,
  VaultStatusVM,
} from './models';
import { reviewImages } from './image-review';

export function useDesktopSettingsState({
  services,
  connection,
  onConnection,
}: {
  services: DesktopServices;
  connection?: ConnectionVM;
  onConnection: (connection: ConnectionVM | undefined) => void;
}) {
  const [platform, setPlatform] = useState<PlatformVM>();
  const [credentials, setCredentials] = useState({
    accessKey: '',
    secretKey: '',
    securityToken: '',
    region: 'cn-beijing',
  });
  const [vaultStatus, setVaultStatus] = useState<VaultStatusVM>({
    exists: false,
  });
  const [vaultPassphrase, setVaultPassphrase] = useState('');
  const [vaultConfirm, setVaultConfirm] = useState('');
  const [vaultUnlockPassphrase, setVaultUnlockPassphrase] = useState('');
  const [showVaultPassphrase, setShowVaultPassphrase] = useState(false);
  const [region, setRegion] = useState('cn-beijing');
  const [regions, setRegions] = useState<RegionVM[]>([
    {
      id: 'cn-beijing',
      name: '华北2（北京）',
      endpointHost: 'agentkit.cn-beijing.volcengineapi.com',
    },
    {
      id: 'cn-shanghai',
      name: '华东2（上海）',
      endpointHost: 'agentkit.cn-shanghai.volcengineapi.com',
    },
  ]);
  const [project, setProject] = useState('default');
  const [runtimes, setRuntimes] = useState<RuntimeVM[]>([]);
  const [directoryScope, setDirectoryScope] = useState('');
  const [directoryLoading, setDirectoryLoading] = useState(false);
  const directoryRequest = useRef(0);
  const [query, setQuery] = useState('');
  const [identity, setIdentity] = useState({
    userId: '',
    applicationToken: '',
    testDeployment: false,
  });
  const [channelAuth, setChannelAuth] = useState({
    mode: 'runtimeKey',
    secret: '',
  });
  const [images, setImages] = useState<ImagesVM>();
  const [review, setReview] = useState<ReturnType<typeof reviewImages>>();
  const [update, setUpdate] = useState<{
    operationId: string;
    status: string;
  }>();
  const [updates, setUpdates] = useState<
    Array<{ operationId: string; status: string }>
  >([]);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const generation = useRef(0);
  useEffect(() => {
    const current = ++generation.current;
    void services.platform
      .status()
      .then((value) => {
        if (generation.current === current) {
          setPlatform(value);
          if (value.region) setRegion(value.region);
        }
      })
      .catch((error: unknown) => {
        if (generation.current === current)
          setError(
            error instanceof Error
              ? error.message
              : 'Could not read account status.',
          );
      });
    if (services.platform.regions) {
      void services.platform
        .regions()
        .then((value) => {
          if (generation.current === current && value.length) setRegions(value);
        })
        .catch(() => {
          // The packaged native catalog remains the safe fallback.
        });
    }
    if (services.platform.vaultStatus) {
      void services.platform
        .vaultStatus()
        .then((value) => {
          if (generation.current === current) setVaultStatus(value);
        })
        .catch(() => {});
    }
    return () => {
      generation.current++;
    };
  }, [services]);
  const loadVaultStatus = useCallback(async () => {
    if (!services.platform.vaultStatus) {
      setVaultStatus({ exists: false });
      return;
    }
    setVaultStatus(await services.platform.vaultStatus());
  }, [services]);
  useLayoutEffect(() => {
    generation.current++;
    setBusy(false);
    setReview(undefined);
    setImages(undefined);
    setUpdate(undefined);
    setUpdates([]);
    if (connection && services.deployment) {
      let active = true;
      void services.deployment
        .updates(connection.runtimeId)
        .then((list) => {
          if (active) setUpdates(list);
        })
        .catch((error: unknown) => {
          if (active)
            setError(
              error instanceof Error
                ? error.message
                : 'Could not read saved update receipts.',
            );
        });
      return () => {
        active = false;
      };
    }
  }, [connection?.epoch]);
  const act = async (run: () => Promise<void>) => {
    const current = generation.current;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await run();
    } catch (error) {
      if (generation.current === current)
        setError(
          error instanceof Error ? error.message : 'Settings action failed.',
        );
    } finally {
      if (generation.current === current) {
        if (connection && services.deployment) {
          try {
            const saved = await services.deployment.updates(
              connection.runtimeId,
            );
            if (generation.current === current) setUpdates(saved);
          } catch {
            if (generation.current === current)
              setNotice(
                'Saved update receipts could not be refreshed. Use Refresh saved receipts to recover the original operation when the host is available.',
              );
          }
        }
        if (generation.current === current) setBusy(false);
      }
    }
  };
  const scope = JSON.stringify([platform?.accountId, region, project]);
  const load = useCallback(
    async (forceRefresh = false) => {
      if (!platform?.signedIn) return;
      const current = ++directoryRequest.current;
      setDirectoryLoading(true);
      try {
        const list = await services.platform.list(
          region,
          project,
          forceRefresh,
        );
        if (directoryRequest.current === current) {
          setRuntimes(list);
          setDirectoryScope(scope);
        }
      } catch (error) {
        if (directoryRequest.current === current) {
          setRuntimes([]);
          setError(
            error instanceof Error
              ? error.message
              : 'Could not load MPA Runtimes.',
          );
        }
      } finally {
        if (directoryRequest.current === current) setDirectoryLoading(false);
      }
    },
    [services, platform?.signedIn, scope, region, project],
  );
  useEffect(() => {
    setRuntimes([]);
    setDirectoryScope('');
    if (platform?.signedIn) void load();
    else setDirectoryLoading(false);
    return () => {
      directoryRequest.current++;
    };
  }, [load, platform?.signedIn]);
  const visibleRuntimes =
    directoryScope === scope
      ? runtimes.filter((runtime) =>
          `${runtime.id} ${runtime.name}`
            .toLowerCase()
            .includes(query.toLowerCase()),
        )
      : [];
  return {
    error,
    notice,
    platform,
    busy,
    act,
    services,
    setPlatform,
    setRuntimes,
    onConnection,
    generation,
    credentials,
    setCredentials,
    vaultStatus,
    setVaultStatus,
    vaultPassphrase,
    setVaultPassphrase,
    vaultConfirm,
    setVaultConfirm,
    vaultUnlockPassphrase,
    setVaultUnlockPassphrase,
    showVaultPassphrase,
    setShowVaultPassphrase,
    loadVaultStatus,
    setRegion,
    setNotice,
    region,
    regions,
    project,
    setProject,
    directoryLoading,
    load,
    query,
    setQuery,
    visibleRuntimes,
    connection,
    identity,
    setIdentity,
    channelAuth,
    setChannelAuth,
    setUpdates,
    setImages,
    images,
    setReview,
    setError,
    review,
    setUpdate,
    updates,
    update,
  };
}
