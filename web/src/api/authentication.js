import API from './api';

export function doLogin(username, password, onSuccess, onError) {
  API.post(`api/login`, {
    username: username,
    password: password
  })
  .then(res => {
    if(res.status !== 200) {
      throw new Error(res.data)
    }
    return res.data;
  }).then(function (data) {
    onSuccess(data);
  }).catch(function (error) {
    if(error.response) {
      onError(error.response.data);
    } else {
      onError({
        message: "Couldn't connect to the API"
      });
    }
  });
}

export function checkIfInstalled(onSuccess, onError) {
  API.get(`api/installed`)
  .then(res => {
    if(res.status !== 200) {
      throw new Error(res.data)
    }
    return res.data;
  }).then(function (data) {
    onSuccess(data);
  }).catch(function (error) {
    if(error.response) {
      onError(error.response.data);
    } else {
      onError({
        message: "Couldn't connect to the API"
      });
    }
  });
}

export function doAuth(onSuccess, onError) {

}

export function doRefreshToken(onSuccess, onError) {

}
