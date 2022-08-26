import API from './api';

export function doGetConfig(onSuccess, onError) {
  const endpoint = API.get(`api/config`);
  endpoint
    .then((res) => {
      if (res.status !== 200) {
        throw new Error(res.data);
      }
      return res.data;
    })
    .then((data) => {
      onSuccess(data);
    })
    .catch((error) => {
      onError(error);
    });
}

export function doSaveConfig(config, onSuccess, onError) {
  const endpoint = API.post(`api/config`, {
    ...config,
  });
  endpoint
    .then((res) => {
      if (res.status !== 200) {
        throw new Error(res.data);
      }
      return res.data;
    })
    .then((data) => {
      onSuccess(data);
    })
    .catch((error) => {
      onError(error);
    });
}

export function doGetKerberosAgentTags(onSuccess, onError) {
  const endpoint = API.get(`api/kerberos-agent/tags`);
  endpoint
    .then((res) => {
      if (res.status !== 200) {
        throw new Error(res.data);
      }
      return res.data;
    })
    .then((data) => {
      onSuccess(data);
    })
    .catch((error) => {
      onError(error);
    });
}

export function doVerifyPersistence(config, onSuccess, onError) {
  const endpoint = API.post(`api/persistence/verify`, {
    ...config,
  });
  endpoint
    .then((res) => {
      if (res.status !== 200) {
        throw new Error(res.data);
      }
      return res.data;
    })
    .then((data) => {
      onSuccess(data);
    })
    .catch((error) => {
      onError(error);
    });
}

export function doVerifyHub(config, onSuccess, onError) {
  const endpoint = API.post(`api/hub/verify`, {
    ...config,
  });
  endpoint
    .then((res) => {
      if (res.status !== 200) {
        throw new Error(res.data);
      }
      return res.data;
    })
    .then((data) => {
      onSuccess(data);
    })
    .catch((error) => {
      onError(error);
    });
}
