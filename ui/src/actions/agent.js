import {
  doGetConfig,
  doSaveConfig,
  doVerifyOnvif,
  doVerifyHub,
  doVerifyPersistence,
  doVerifySecondaryPersistence,
  doGetKerberosAgentTags,
  doGetDashboardInformation,
  doGetEvents,
  doVerifyCamera,
} from '../api/agent';

export const addRegion = (id, polygon) => {
  return (dispatch) => {
    dispatch({
      type: 'ADD_REGION',
      id,
      polygon,
    });
  };
};

export const removeRegion = (id, polygon) => {
  return (dispatch) => {
    dispatch({
      type: 'REMOVE_REGION',
      id,
      polygon,
    });
  };
};

export const updateRegion = (id, polygon) => {
  return (dispatch) => {
    dispatch({
      type: 'UPDATE_REGION',
      id,
      polygon,
    });
  };
};

export const verifyOnvif = (config, onSuccess, onError) => {
  return (dispatch) => {
    doVerifyOnvif(
      config,
      (data) => {
        dispatch({
          type: 'VERIFY_ONVIF',
        });
        if (onSuccess) {
          onSuccess(data);
        }
      },
      (error) => {
        const { data } = error;
        if (onError) {
          onError(data);
        }
      }
    );
  };
};

export const verifyCamera = (streamType, config, onSuccess, onError) => {
  return (dispatch) => {
    doVerifyCamera(
      streamType,
      config,
      () => {
        dispatch({
          type: 'VERIFY_CAMERA',
        });
        if (onSuccess) {
          onSuccess();
        }
      },
      (error) => {
        const { message } = error.response.data;
        if (onError) {
          onError(message);
        }
      }
    );
  };
};

export const verifyPersistence = (config, onSuccess, onError) => {
  return (dispatch) => {
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

export const verifySecondaryPersistence = (config, onSuccess, onError) => {
  return (dispatch) => {
    doVerifySecondaryPersistence(
      config,
      () => {
        dispatch({
          type: 'VERIFY_SECONDARY_PERSISTENCE',
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
  return (dispatch) => {
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
  return (dispatch) => {
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

export const getDashboardInformation = (onSuccess, onError) => {
  return (dispatch) => {
    doGetDashboardInformation(
      (data) => {
        dispatch({
          type: 'GET_DASHBOARD',
          dashboard: data,
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

export const getEvents = (eventfilter, onSuccess, onError) => {
  return (dispatch) => {
    doGetEvents(
      eventfilter,
      (data) => {
        dispatch({
          type: 'GET_EVENTS',
          events: data.events,
          filter: eventfilter,
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

export const getConfig = (onSuccess, onError) => {
  return (dispatch) => {
    doGetConfig(
      (config) => {
        dispatch({
          type: 'GET_CONFIG',
          config,
        });
        if (onSuccess) {
          onSuccess(config);
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

export const updateConfig = (field, value) => {
  return (dispatch) => {
    dispatch({
      type: 'UPDATE_CONFIG',
      field,
      value,
    });
  };
};

export const saveConfig = (service, config, onSuccess, onError) => {
  return (dispatch) => {
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
