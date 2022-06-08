import React from 'react';
import PropTypes from 'prop-types';
import {
  Block,
  Button,
  BlockHeader,
  BlockBody,
  BlockFooter,
  AlertMessage,
  Input,
  Icon,
  LandingLayout,
} from '@kerberos-io/ui';
import { withRouter } from 'react-router-dom';
import { connect } from 'react-redux';
import { login, resetLogin } from '../../actions';
import config from '../../config';
import './Login.module.scss';

class Login extends React.Component {
  constructor() {
    super();
    this.handleSubmit = this.handleSubmit.bind(this);
    this.hideMessage = this.hideMessage.bind(this);
    this.togglePasswordVisible = this.togglePasswordVisible.bind(this);
  }

  handleSubmit(event) {
    event.preventDefault();
    const { dispatchLogin } = this.props;
    const { target } = event;
    const data = new FormData(target);
    dispatchLogin(data.get('username'), data.get('password'));
  }

  togglePasswordVisible() {
    const { passwordVisible } = this.state;
    this.setState({
      passwordVisible: !passwordVisible,
    });
  }

  hideMessage() {
    const { dispatchResetLogin } = this.props;
    dispatchResetLogin();
  }

  render() {
    const { loginError, error } = this.props;

    return (
      <LandingLayout
        title="Kerberos Agent"
        version={config.VERSION}
        description="Scale your Kerberos Agents"
      >
        <section className="login-body">
          <Block>
            <form onSubmit={this.handleSubmit} noValidate>
              <BlockHeader>
                <div>
                  <Icon label="login" /> <h4>Login</h4>
                </div>
              </BlockHeader>
              {loginError && (
                <AlertMessage
                  message={error}
                  onClick={() => this.hideMessage()}
                />
              )}
              <BlockBody>
                <Input
                  label="username or email"
                  placeholder="Your username/email"
                  readonly={false}
                  disabled={false}
                  type="text"
                  name="username"
                  iconleft="accounts"
                />
                <Input
                  label="password"
                  placeholder="Your password"
                  readonly={false}
                  disabled={false}
                  type="password"
                  name="password"
                  iconleft="locked"
                  iconright="activity"
                  seperate
                />
              </BlockBody>
              <BlockFooter>
                <p />
                <Button
                  buttonType="submit"
                  type="submit"
                  icon="logout"
                  label="Login"
                />
              </BlockFooter>
            </form>
          </Block>
        </section>
      </LandingLayout>
    );
  }
}

const mapStateToProps = (state) => ({
  loginError: state.auth.loginError,
  error: state.auth.error,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchLogin: (username, password) => {
    dispatch(login(username, password));
  },
  dispatchResetLogin: () => {
    dispatch(resetLogin());
  },
});

Login.propTypes = {
  loginError: PropTypes.bool.isRequired,
  error: PropTypes.string.isRequired,
  dispatchLogin: PropTypes.func.isRequired,
  dispatchResetLogin: PropTypes.func.isRequired,
};

export default withRouter(connect(mapStateToProps, mapDispatchToProps)(Login));
