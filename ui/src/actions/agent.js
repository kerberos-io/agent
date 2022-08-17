import {
  doGetConfig,
  doSaveConfig,
  doVerifyHub,
  doVerifyPersistence,
  doGetKerberosAgentTags,
} from '../api/agent';

export const verifyPersistence = (config, onSuccess, onError) => {
  return function (dispatch) {
    doVerifyPersistence(
      config,
      () => {
        dispatch({
          type: 'VERIFY_PERSISTENCE',
        });
        if (onSuccess) {
          onSuccess();
        }
      },
      (error) => {
        const { data } = error.response.data;
        if (onError) {
          onError(data);
        }
      }
    );
  };
};

export const verifyHub = (config, onSuccess, onError) => {
  return function (dispatch) {
    doVerifyHub(
      config,
      () => {
        dispatch({
          type: 'VERIFY_HUB',
        });
        if (onSuccess) {
          onSuccess();
        }
      },
      (error) => {
        const { data } = error.response.data;
        if (onError) {
          onError(data);
        }
      }
    );
  };
};

export const getKerberosAgentTags = (onSuccess, onError) => {
  return function (dispatch) {
    doGetKerberosAgentTags(
      (data) => {
        dispatch({
          type: 'GET_MACHINERY_TAGS',
          tags: data.data,
        });
        if (onSuccess) {
          onSuccess();
        }
      },
      () => {
        if (onError) {
          onError();
        }
      }
    );
  };
};

export const getConfig = (service, onSuccess, onError) => {
  return function (dispatch) {
    doGetConfig(
      service,
      (data) => {
        dispatch({
          type: 'GET_CONFIG',
          data,
        });
        if (onSuccess) {
          onSuccess();
        }
      },
      () => {
        if (onError) {
          onError();
        }
      }
    );
  };
};

export const saveConfig = (service, config, onSuccess, onError) => {
  return function (dispatch) {
    doSaveConfig(
      service,
      config,
      () => {
        dispatch({
          type: 'SAVE_CONTAINER',
        });
        if (onSuccess) {
          onSuccess();
        }
      },
      () => {
        if (onError) {
          onError();
        }
      }
    );
  };
};
