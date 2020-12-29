import React from 'react';
import { connect } from 'react-redux';
import { withRouter } from 'react-router-dom';
import { login } from '../../actions';
import Button from '@material-ui/core/Button';
import TextField from '@material-ui/core/TextField';
import Typography from '@material-ui/core/Typography';
import './LoginForm.css';

class LoginForm extends React.Component {

  constructor() {
    super();
    this.handleSubmit = this.handleSubmit.bind(this)
  }

  handleSubmit(event) {
    event.preventDefault();
    const data = new FormData(event.target);
    this.props.dispatchLogin(data.get('username'), data.get('password'));
  }

  render() {
    const { loginError, error } = this.props;

    return <div className="paper-loginform">

        { loginError && <span className="error">{ error }</span> }

        { loginError && this.isInvalidLicense(error) && <>
            <Typography className="login-title" component="h1" variant="h5">
              Update License key
            </Typography>
            <form className="form" onSubmit={this.updateLicenseKey} noValidate>
              <TextField
                variant="outlined"
                margin="normal"
                required
                fullWidth
                id="licensekey"
                label="Licensekey"
                name="licensekey"
              />
              <Button
                type="submit"
                fullWidth
                variant="contained"
                color="primary"
                className="signin-button"
              >
                Update
              </Button>
            </form>
          </>
        }
    </div>
  }
}

const mapStateToProps = (state, ownProps) => ({
  loginError: state.auth.loginError,
  error: state.auth.error,
})

const mapDispatchToProps = (dispatch, ownProps) => ({
  dispatchLogin: (username, password) => {
    dispatch(login(username, password))
  },
})

export default withRouter(connect(mapStateToProps, mapDispatchToProps)(LoginForm));
