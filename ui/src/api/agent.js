import API from './api';

export function doGetConfig(onSuccess, onError) {
  const endpoint = API.get(`config`);
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
  const endpoint = API.post(`config`, {
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
  const endpoint = API.get(`kerberos-agent/tags`);
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
  const endpoint = API.post(`persistence/verify`, {
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
  const endpoint = API.post(`hub/verify`, {
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

export function doGenerateKeys(onSuccess, onError) {
  const endpoint = API.post(`keys/generate`);
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

export function doVerifyCamera(streamType, config, onSuccess, onError) {
  const cameraStreams = {
    rtsp: '',
    sub_rtsp: '',
  };

  if (config) {
    cameraStreams.rtsp = config.capture.ipcamera.rtsp;
    cameraStreams.sub_rtsp = config.capture.ipcamera.sub_rtsp;
  }

  const endpoint = API.post(`camera/verify/${streamType}`, cameraStreams);
  endpoint
    .then((res) => {
      if (res.status !== 200) {
        throw new Error(res.message);
      }
      return res.message;
    })
    .then((data) => {
      onSuccess(data);
    })
    .catch((error) => {
      onError(error);
    });
}

export function doGetDashboardInformation(onSuccess, onError) {
  const endpoint = API.get(`dashboard`);
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

export function doGetEvents(eventfilter, onSuccess, onError) {
  const endpoint = API.post(`latest-events`, eventfilter);
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
